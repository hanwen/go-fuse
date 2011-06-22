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

// fileBridge stores either an open dir or an open file.
type fileBridge struct {
	*mountData
	*inode
	Flags uint32
	Iface interface{}
}

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

	// Protects parent/child relations within the mount.
	// treeLock should be acquired before openFilesLock
	treeLock sync.RWMutex

	// Protects openFiles
	openFilesLock sync.RWMutex

	// Open files/directories.
	openFiles map[uint64]*fileBridge
}

func newMount(fs FileSystem) *mountData {
	return &mountData{
		fs:        fs,
		openFiles: make(map[uint64]*fileBridge),
	}
}

func (me *mountData) setOwner(attr *Attr) {
	if me.options.Owner != nil {
		attr.Owner = *me.options.Owner
	}
}
func (me *mountData) unregisterFile(node *inode, handle uint64) interface{} {
	me.openFilesLock.Lock()
	defer me.openFilesLock.Unlock()
	b, ok := me.openFiles[handle]
	if !ok {
		panic("invalid handle")
	}
	node.OpenCount--
	me.openFiles[handle] = nil, false
	return b.Iface
}

func (me *mountData) registerFile(node *inode, f interface{}, flags uint32) uint64 {
	me.openFilesLock.Lock()
	defer me.openFilesLock.Unlock()

	b := &fileBridge{
		Iface:     f,
		inode:     node,
		mountData: me,
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

////////////////

// Tests should set to true.
var paranoia = false

// TODO should rename to dentry?
type inode struct {
	Parent      *inode
	Children    map[string]*inode
	NodeId      uint64
	Name        string
	LookupCount int

	// Protected by openFilesLock.
	// TODO - verify() this variable too.
	OpenCount int

	// Non-nil if this is a mountpoint.
	mountPoint *mountData

	// The point under which this node is.  Should be non-nil for
	// all nodes.
	mount *mountData
}

// TotalOpenCount counts open files.  It should only be entered from
// an inode which is a mountpoint.
func (me *inode) TotalOpenCount() int {
	o := 0
	if me.mountPoint != nil {
		me.mountPoint.treeLock.RLock()
		defer me.mountPoint.treeLock.RUnlock()

		me.mountPoint.openFilesLock.RLock()
		defer me.mountPoint.openFilesLock.RUnlock()

		o += len(me.mountPoint.openFiles)
	}

	for _, v := range me.Children {
		o += v.TotalOpenCount()
	}
	return o
}

// TotalMountCount counts mountpoints.  It should only be entered from
// an inode which is a mountpoint.
func (me *inode) TotalMountCount() int {
	o := 0
	if me.mountPoint != nil {
		if me.mountPoint.unmountPending {
			return 0
		}

		o++
		me.mountPoint.treeLock.RLock()
		defer me.mountPoint.treeLock.RUnlock()
	}

	for _, v := range me.Children {
		o += v.TotalMountCount()
	}
	return o
}

func (me *inode) IsDir() bool {
	return me.Children != nil
}

const initDirSize = 20

func (me *inode) verify(cur *mountData) {
	if !(me.NodeId == FUSE_ROOT_ID || me.LookupCount > 0 || len(me.Children) > 0 || me.mountPoint != nil) {
		p, _ := me.GetPath()
		panic(fmt.Sprintf("node %v %d should be dead: %v %v", p, me.NodeId, len(me.Children), me.LookupCount))
	}
	if me.mountPoint != nil {
		if me.mountPoint.unmountPending && len(me.mountPoint.openFiles) > 0 {
			panic(fmt.Sprintf("cannot have open files for pending unmount"))
		}
		cur = me.mountPoint
	}
	if me.mount != cur {
		panic(fmt.Sprintf("me.mount not set correctly %v %v", me.mount, cur))
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
		ch.verify(cur)
	}
}

func (me *inode) GetFullPath() (path string) {
	rev_components := make([]string, 0, 10)
	inode := me
	for ; inode != nil; inode = inode.Parent {
		rev_components = append(rev_components, inode.Name)
	}
	return ReverseJoin(rev_components, "/")
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.
func (me *inode) GetPath() (path string, mount *mountData) {
	me.mount.treeLock.RLock()
	defer me.mount.treeLock.RUnlock()
		
	if me.NodeId != FUSE_ROOT_ID && me.Parent == nil {
		// Deleted node.  Treat as if the filesystem was unmounted.
		return ".deleted", nil
	}

	rev_components := make([]string, 0, 10)
	inode := me

	for ; inode != nil && inode.mountPoint == nil; inode = inode.Parent {
		rev_components = append(rev_components, inode.Name)
	}
	if inode == nil {
		panic(fmt.Sprintf("did not find parent with mount: %v", rev_components))
	}
	mount = inode.mountPoint

	if mount.unmountPending {
		return "", nil
	}
	return ReverseJoin(rev_components, "/"), mount
}

// Must be called with treeLock for the mount held.
func (me *inode) setParent(newParent *inode) {
	oldParent := me.Parent
	if oldParent == newParent {
		return
	}
	if oldParent != nil {
		if paranoia {
			ch := oldParent.Children[me.Name]
			if ch == nil {
				panic(fmt.Sprintf("parent has no child named %v", me.Name))
			}
		}
		oldParent.Children[me.Name] = nil, false

		if oldParent.mountPoint != nil && oldParent.mountPoint.unmountPending &&
			len(oldParent.Children) == 0 {
			oldParent.mountPoint = nil
			if oldParent.Parent != nil {
				oldParent.mount = oldParent.Parent.mount
			}
		}

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
		Owner:           CurrentOwner(),
	}
}

type FileSystemConnector struct {
	DefaultRawFileSystem

	Debug bool

	////////////////

	// inodeMapMutex protects inodeMap
	inodeMapMutex sync.Mutex

	// Invariants: see the verify() method.
	inodeMap map[uint64]*inode
	rootNode *inode
}

func (me *FileSystemConnector) Statistics() string {
	root := me.rootNode
	me.inodeMapMutex.Lock()
	defer me.inodeMapMutex.Unlock()
	return fmt.Sprintf("Mounts %20d\nFiles %20d\nInodes %20d\n",
		root.TotalMountCount(),
		root.TotalOpenCount(), len(me.inodeMap))
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
	me.inodeMapMutex.Lock()
	defer me.inodeMapMutex.Unlock()

	for k, v := range me.inodeMap {
		if v.NodeId != k {
			panic(fmt.Sprintf("nodeid mismatch %v %v", v, k))
		}
	}

	root := me.rootNode
	root.verify(me.rootNode.mountPoint)
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

func (me *FileSystemConnector) lookupUpdate(parent *inode, name string, isDir bool, lookupCount int) *inode {
	defer me.verify()

	parent.mount.treeLock.Lock()
	defer parent.mount.treeLock.Unlock()

	data, ok := parent.Children[name]
	if !ok {
		data = me.newInode(false, isDir)
		data.Name = name
		data.setParent(parent)
		data.mount = parent.mount
	}
	data.LookupCount += lookupCount
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

	node := me.getInodeData(nodeId)

	node.LookupCount -= forgetCount
	me.considerDropInode(node)
}

func (me *FileSystemConnector) considerDropInode(n *inode) {
	n.mount.treeLock.Lock()
	defer n.mount.treeLock.Unlock()

	dropInode := n.LookupCount <= 0 && len(n.Children) == 0 &&
		(n.mountPoint == nil || n.mountPoint.unmountPending) &&
		n.OpenCount <= 0
	if dropInode {
		n.setParent(nil)

		me.inodeMapMutex.Lock()
		defer me.inodeMapMutex.Unlock()
		me.inodeMap[n.NodeId] = nil, false
	}
}

func (me *FileSystemConnector) renameUpdate(oldParent *inode, oldName string, newParent *inode, newName string) {
	defer me.verify()
	if oldParent.mount != newParent.mount {
		panic("Cross mount rename")
	}
	oldParent.mount.treeLock.Lock()
	defer oldParent.mount.treeLock.Unlock()

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

	parent.mount.treeLock.Lock()
	defer parent.mount.treeLock.Unlock()

	node := parent.Children[name]
	node.setParent(nil)
	node.Name = ".deleted"
}

// Walk the file system starting from the root. Will return nil if
// node not found.
func (me *FileSystemConnector) findInode(fullPath string) *inode {
	fullPath = strings.TrimLeft(filepath.Clean(fullPath), "/")
	comps := strings.Split(fullPath, "/", -1)

	node := me.rootNode
	for _, component := range comps {
		if len(component) == 0 {
			continue
		}

		if node.mountPoint != nil {
			node.mountPoint.treeLock.RLock()
			defer node.mountPoint.treeLock.RUnlock()
		}

		node = node.Children[component]
		if node == nil {
			return nil
		}
	}
	return node
}

////////////////////////////////////////////////////////////////

func EmptyFileSystemConnector() (out *FileSystemConnector) {
	out = new(FileSystemConnector)
	out.inodeMap = make(map[uint64]*inode)

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
		if dirParentNode == nil {
			log.Println("Could not find mountpoint:", mountPoint)
			return ENOENT
		}
		// Make sure we know the mount point.
		_, _, node = me.internalLookupWithNode(dirParentNode, base, 0)
	} else {
		node = me.rootNode
	}
	if node == nil {
		log.Println("Could not find mountpoint:", mountPoint)
		return ENOENT
	}

	if !node.IsDir() {
		return EINVAL
	}

	if node != me.rootNode {
		node.mount.treeLock.Lock()
		defer node.mount.treeLock.Unlock()
	}

	hasChildren := len(node.Children) > 0
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

	node.mountPoint = newMount(fs)
	node.mount = node.mountPoint
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	node.mountPoint.options = opts
	return OK
}

func (me *FileSystemConnector) Unmount(path string) Status {
	node := me.findInode(path)
	if node == nil {
		log.Println("Could not find mountpoint:", path)
		return EINVAL
	}

	// Need to lock to look at node.Children
	unmountError := OK

	mount := node.mountPoint
	if mount == nil || mount.unmountPending {
		unmountError = EINVAL
	}

	// don't use defer: we don't want to call out to
	// mount.fs.Unmount() with lock held.
	if unmountError.Ok() && (node.TotalOpenCount() > 0 || node.TotalMountCount() > 1) {
		unmountError = EBUSY
	}

	if unmountError.Ok() {
		// We settle for eventual consistency.
		mount.unmountPending = true
	}

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

	p, m := n.GetPath()
	if me.Debug {
		log.Printf("Node %v = '%s'", nodeid, n.GetFullPath())
	}

	return p, m, n
}

func (me *FileSystemConnector) getOpenFileData(nodeid uint64, fh uint64) (f File, m *mountData, p string) {
	if fh != 0 {
		var bridge *fileBridge
		f, bridge = me.getFile(fh)
		m = bridge.mountData
	}
	node := me.getInodeData(nodeid)
	node.mount.treeLock.RLock()
	defer node.mount.treeLock.RUnlock()

	path, maybeNil := node.GetPath()
	// If the file was deleted, GetPath() will return nil.
	if maybeNil != nil {
		if m != nil && maybeNil != m {
			panic("mount mismatch")
		}

		m = maybeNil
		p = path
	}
	return
}
