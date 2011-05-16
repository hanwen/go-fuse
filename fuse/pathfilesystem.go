package fuse

/*

 FilesystemConnector is a lowlevel FUSE filesystem that translates
 from inode numbers (as delivered by the kernel) to traditional path
 names.  The paths are then used as arguments for methods of
 FileSystem instances.

 FileSystemConnector supports mounts of different FileSystems
 on top of each other's directories.

 General todos:

 - We are doing lookups (incurring GetAttr() costs) for internal
 lookups (eg. after doing a symlink).  We could probably do without
 the GetAttr calls.

*/

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

type mountData struct {
	// If non-nil the file system mounted here.
	fs FileSystem

	// If yes, we are looking to unmount the mounted fs.
	//
	// To be technically correct, we'd have to have a mutex
	// protecting this.  We don't, keeping the following in mind:
	//
	//  * eventual consistency is OK here
	//
	//  * the kernel controls when to ask for updates,
	//  so we can't make entries disappear directly anyway.
	unmountPending bool

	// We could have separate treeLocks per mount; something to
	// consider if we can measure significant contention for
	// multi-mount filesystems.

	options *FileSystemOptions
}

func newMount(fs FileSystem) *mountData {
	return &mountData{fs: fs}
}

// Tests should set to true.
var paranoia = false

// TODO should rename to dentry?
type inode struct {
	Parent      *inode
	Children    map[string]*inode
	NodeId      uint64
	Name        string
	LookupCount int
	OpenCount   int
	mount       *mountData
}

// Should be called with treeLock and fileLock held.
func (me *inode) totalOpenCount() int {
	o := me.OpenCount
	for _, v := range me.Children {
		o += v.totalOpenCount()
	}
	return o
}

// Should be called with treeLock held.
func (me *inode) totalMountCount() int {
	o := 0
	if me.mount != nil && !me.mount.unmountPending {
		o++
	}
	for _, v := range me.Children {
		o += v.totalMountCount()
	}
	return o
}

func (me *inode) IsDir() bool {
	return me.Children != nil
}

const initDirSize = 20

func (me *inode) verify() {
	if !(me.NodeId == FUSE_ROOT_ID || me.LookupCount > 0 || len(me.Children) > 0) {
		panic(fmt.Sprintf("node should be dead: %v", me))
	}
	for n, ch := range me.Children {
		if ch == nil {
			panic("Found nil child.")
		}
		if ch.Name != n {
			panic(fmt.Sprintf("parent/child name corrupted %v %v",
				ch.Name, n))
		}
		if ch.Parent != me {
			panic(fmt.Sprintf("parent/child relation corrupted %v %v %v",
				ch.Parent, me, ch))
		}
	}
}

func (me *inode) GetPath() (path string, mount *mountData) {
	rev_components := make([]string, 0, 10)
	inode := me

	for ; inode != nil && inode.mount == nil; inode = inode.Parent {
		rev_components = append(rev_components, inode.Name)
	}
	if inode == nil {
		panic(fmt.Sprintf("did not find parent with mount: %v", rev_components))
	}
	mount = inode.mount

	if mount.unmountPending {
		return "", nil
	}
	components := make([]string, len(rev_components))
	for i, v := range rev_components {
		components[len(rev_components)-i-1] = v
	}
	fullPath := strings.Join(components, "/")
	return fullPath, mount
}

// Must be called with treeLock held.
func (me *inode) setParent(newParent *inode) {
	if me.Parent == newParent {
		return
	}
	if me.Parent != nil {
		if paranoia {
			ch := me.Parent.Children[me.Name]
			if ch == nil {
				panic(fmt.Sprintf("parent has no child named %v", me.Name))
			}
		}
		me.Parent.Children[me.Name] = nil, false
		me.Parent = nil
	}
	if newParent != nil {
		me.Parent = newParent

		if paranoia {
			ch := me.Parent.Children[me.Name]
			if ch != nil {
				panic(fmt.Sprintf("Already have an inode with same name: %v: %v", me.Name, ch))
			}
		}

		me.Parent.Children[me.Name] = me
	}
}

func NewFileSystemOptions() *FileSystemOptions {
	return &FileSystemOptions{
		NegativeTimeout: 0.0,
		AttrTimeout:     1.0,
		EntryTimeout:    1.0,
	}
}

type FileSystemConnector struct {
	DefaultRawFileSystem

	Debug bool

	////////////////

	// Protects the inodeMap and each node's Children/Parent
	// relations.
	treeLock sync.RWMutex

	// Invariants: see the verify() method.
	inodeMap map[uint64]*inode
	rootNode *inode

	// Open files/directories.
	openFiles map[uint64]*fileBridge

	// Protects openFiles and OpenCount in all of the nodes.
	fileLock sync.RWMutex
}

type fileBridge struct {
	*mountData
	*inode
	Flags uint32
	Iface interface{}
}

func (me *FileSystemConnector) Statistics() string {
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	me.fileLock.RLock()
	defer me.fileLock.RUnlock()

	root := me.rootNode
	return fmt.Sprintf("Mounts %20d\nFiles %20d\nInodes %20d\n",
		root.totalMountCount(),
		len(me.openFiles), len(me.inodeMap))
}

func (me *FileSystemConnector) unregisterFile(node *inode, handle uint64) interface{} {
	me.fileLock.Lock()
	defer me.fileLock.Unlock()
	b, ok := me.openFiles[handle]
	if !ok {
		panic("invalid handle")
	}
	me.openFiles[handle] = nil, false
	node.OpenCount--
	return b.Iface
}

func (me *FileSystemConnector) registerFile(node *inode, mount *mountData, f interface{}, flags uint32) uint64 {
	me.fileLock.Lock()
	defer me.fileLock.Unlock()

	b := &fileBridge{
		Iface:     f,
		inode:     node,
		mountData: mount,
		Flags:     flags,
	}
	h := uint64(uintptr(unsafe.Pointer(b)))
	_, ok := me.openFiles[h]
	if ok {
		panic("handle counter wrapped")
	}

	node.OpenCount++
	me.openFiles[h] = b
	return h
}

func (me *FileSystemConnector) decodeFileHandle(h uint64) *fileBridge {
	b := (*fileBridge)(unsafe.Pointer(uintptr(h)))
	return b
}

type rawDir interface {
	ReadDir(input *ReadIn) (*DirEntryList, Status)
	Release()
}

func (me *FileSystemConnector) getDir(h uint64) (dir rawDir, bridge *fileBridge) {
	b := me.decodeFileHandle(h)
	return b.Iface.(rawDir), b
}

func (me *FileSystemConnector) getFile(h uint64) (file File, bridge *fileBridge) {
	b := me.decodeFileHandle(h)
	return b.Iface.(File), b
}

func (me *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	me.treeLock.Lock()
	defer me.treeLock.Unlock()
	me.fileLock.Lock()
	defer me.fileLock.Unlock()

	hiddenOpen := 0
	for k, v := range me.inodeMap {
		if v.NodeId != k {
			panic(fmt.Sprintf("nodeid mismatch %v %v", v, k))
		}
		if v.Parent == nil && v != me.rootNode {
			hiddenOpen += v.OpenCount
		}
	}

	root := me.rootNode
	root.verify()

	open := root.totalOpenCount()
	openFiles := len(me.openFiles)
	if open+hiddenOpen != openFiles {
		panic(fmt.Sprintf("opencount mismatch totalOpen=%v openFiles=%v hiddenOpen=%v", open, openFiles, hiddenOpen))
	}
}

func (me *FileSystemConnector) newInode(root bool, isDir bool) *inode {
	data := new(inode)
	if root {
		data.NodeId = FUSE_ROOT_ID
		me.rootNode = data
	} else {
		data.NodeId = uint64(uintptr(unsafe.Pointer(data)))
	}
	me.inodeMap[data.NodeId] = data
	if isDir {
		data.Children = make(map[string]*inode, initDirSize)
	}

	return data
}

func (me *FileSystemConnector) lookupUpdate(parent *inode, name string, isDir bool) *inode {
	defer me.verify()

	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	data, ok := parent.Children[name]
	if !ok {
		data = me.newInode(false, isDir)
		data.Name = name
		data.setParent(parent)
	}

	return data
}

func (me *FileSystemConnector) getInodeData(nodeid uint64) *inode {
	if nodeid == FUSE_ROOT_ID {
		return me.rootNode
	}

	return (*inode)(unsafe.Pointer(uintptr(nodeid)))
}

func (me *FileSystemConnector) forgetUpdate(nodeId uint64, forgetCount int) {
	defer me.verify()
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	data, ok := me.inodeMap[nodeId]
	if ok {
		data.LookupCount -= forgetCount
		me.considerDropInode(data)
	}
}

func (me *FileSystemConnector) considerDropInode(n *inode) {
	if n.LookupCount <= 0 && len(n.Children) == 0 && (n.mount == nil || n.mount.unmountPending) &&
		n.OpenCount <= 0 {
		n.setParent(nil)
		me.inodeMap[n.NodeId] = nil, false
	}
}

func (me *FileSystemConnector) renameUpdate(oldParent *inode, oldName string, newParent *inode, newName string) {
	defer me.verify()
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	node := oldParent.Children[oldName]
	if node == nil {
		panic("Source of rename does not exist")
	}

	dest := newParent.Children[newName]
	if dest != nil {
		dest.setParent(nil)
	}
	node.setParent(nil)
	node.Name = newName
	node.setParent(newParent)
}

func (me *FileSystemConnector) unlinkUpdate(parent *inode, name string) {
	defer me.verify()
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	node := parent.Children[name]
	node.setParent(nil)
	node.Name = ".deleted"
}

// Walk the file system starting from the root.
func (me *FileSystemConnector) findInode(fullPath string) *inode {
	fullPath = strings.TrimLeft(filepath.Clean(fullPath), "/")
	comps := strings.Split(fullPath, "/", -1)

	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	node := me.rootNode
	for i, component := range comps {
		if len(component) == 0 {
			continue
		}

		node = node.Children[component]
		if node == nil {
			panic(fmt.Sprintf("findInode: %v %v", i, fullPath))
		}
	}
	return node
}

////////////////////////////////////////////////////////////////

func EmptyFileSystemConnector() (out *FileSystemConnector) {
	out = new(FileSystemConnector)
	out.inodeMap = make(map[uint64]*inode)
	out.openFiles = make(map[uint64]*fileBridge)

	rootData := out.newInode(true, true)
	rootData.Children = make(map[string]*inode, initDirSize)

	out.verify()
	return out
}

func (me *FileSystemConnector) Mount(mountPoint string, fs FileSystem, opts *FileSystemOptions) Status {
	var node *inode

	if mountPoint != "/" {
		dirParent, base := filepath.Split(mountPoint)
		dirParentNode := me.findInode(dirParent)

		// Make sure we know the mount point.
		_, _, node = me.internalLookupWithNode(dirParentNode, base, 0)
	} else {
		node = me.rootNode
	}
	if node == nil {
		log.Println("Could not find mountpoint?", mountPoint)
		return ENOENT
	}

	if !node.IsDir() {
		return EINVAL
	}

	me.treeLock.RLock()
	hasChildren := len(node.Children) > 0
	// don't use defer, as we dont want to hold the lock during
	// fs.Mount().
	me.treeLock.RUnlock()

	if hasChildren {
		return EBUSY
	}

	code := fs.Mount(me)
	if code != OK {
		log.Println("Mount error: ", mountPoint, code)
		return code
	}

	if me.Debug {
		log.Println("Mount: ", fs, "on", mountPoint, node)
	}

	node.mount = newMount(fs)
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	node.mount.options = opts
	return OK
}

func (me *FileSystemConnector) Unmount(path string) Status {
	node := me.findInode(path)
	if node == nil {
		panic(path)
	}

	// Need to lock to look at node.Children
	me.treeLock.RLock()
	me.fileLock.Lock()

	unmountError := OK

	mount := node.mount
	if mount == nil || mount.unmountPending {
		unmountError = EINVAL
	}

	// don't use defer: we don't want to call out to
	// mount.fs.Unmount() with lock held.
	if unmountError.Ok() && (node.totalOpenCount() > 0 || node.totalMountCount() > 1) {
		unmountError = EBUSY
	}

	if unmountError.Ok() {
		// We settle for eventual consistency.
		mount.unmountPending = true
	}
	me.fileLock.Unlock()
	me.treeLock.RUnlock()

	if unmountError.Ok() {
		if me.Debug {
			log.Println("Unmount: ", mount)
		}

		mount.fs.Unmount()
	}
	return unmountError
}

func (me *FileSystemConnector) GetPath(nodeid uint64) (path string, mount *mountData, node *inode) {
	n := me.getInodeData(nodeid)

	// Need to lock because renames create invalid states.
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	p, m := n.GetPath()
	if me.Debug {
		log.Printf("Node %v = '%s'", nodeid, p)
	}

	return p, m, n
}

func (me *FileSystemConnector) getOpenFileData(nodeid uint64, fh uint64) (f File, m *mountData, p string) {
	if fh != 0 {
		var bridge *fileBridge
		f, bridge = me.getFile(fh)
		m = bridge.mountData
	}
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	node := me.getInodeData(nodeid)
	if node.Parent != nil {
		p, m = node.GetPath()
	}

	return
}
