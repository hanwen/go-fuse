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

// openedFile stores either an open dir or an open file.
type openedFile struct {
	Handled
	*mountData
	*inode
	Flags uint32

	dir  rawDir
	file File
}

type mountData struct {
	// If non-nil the file system mounted here.
	fs FileSystem

	// Node that we were mounted on.
	mountPoint *inode

	// We could have separate treeLocks per mount; something to
	// consider if we can measure significant contention for
	// multi-mount filesystems.
	options *FileSystemOptions

	// Protects parent/child relations within the mount.
	// treeLock should be acquired before openFilesLock
	treeLock sync.RWMutex

	// Manage filehandles of open files.
	openFiles *HandleMap
}

func newMount(fs FileSystem) *mountData {
	return &mountData{
		fs:        fs,
		openFiles: NewHandleMap(),
	}
}

func (me *mountData) setOwner(attr *Attr) {
	if me.options.Owner != nil {
		attr.Owner = *me.options.Owner
	}
}

func (me *mountData) unregisterFileHandle(node *inode, handle uint64) *openedFile {
	obj := me.openFiles.Forget(handle)
	opened := (*openedFile)(unsafe.Pointer(obj))

	node.OpenCountMutex.Lock()
	defer node.OpenCountMutex.Unlock()
	node.OpenCount--

	return opened
}

func (me *mountData) registerFileHandle(node *inode, dir rawDir, f File, flags uint32) uint64 {
	node.OpenCountMutex.Lock()
	defer node.OpenCountMutex.Unlock()
	b := &openedFile{
		dir:       dir,
		file:      f,
		inode:     node,
		mountData: me,
		Flags:     flags,
	}
	node.OpenCount++
	return me.openFiles.Register(&b.Handled)
}

////////////////

// Tests should set to true.
var paranoia = false

// The inode is a combination of dentry (entry in the file/directory
// tree) and inode.  We do this, since in the high-level API, each
// files and inodes correspond one-to-one.
type inode struct {
	Handled

	// Constant during lifetime.
	NodeId      uint64

	// Number of open files and its protection.
	OpenCountMutex sync.Mutex
	OpenCount      int

	// me.mount.treeLock; we need store this mutex separately,
	// since unmount may set me.mount = nil during Unmount().
	// Constant during lifetime.
	//
	// If multiple treeLocks must be acquired, the treeLocks
	// closer to the root must be acquired first.
	treeLock *sync.RWMutex

	// All data below is protected by treeLock.
	Name        string
	Parent      *inode
	Children    map[string]*inode
	Mounts      map[string]*mountData
	LookupCount int

	// Non-nil if this is a mountpoint.
	mountPoint *mountData

	// The file system to which this node belongs.  Is constant
	// during the lifetime, except upon Unmount() when it is set
	// to nil.
	mount *mountData
}

// Must be called with treeLock held.
func (me *inode) canUnmount() bool {
	for _, v := range me.Children {
		if v.mountPoint != nil {
			// This access may be out of date, but it is no
			// problem to err on the safe side.
			return false
		}
		if !v.canUnmount() {
			return false
		}
	}

	me.OpenCountMutex.Lock()
	defer me.OpenCountMutex.Unlock()
	return me.OpenCount == 0
}

// Must be called with treeLock held
func (me *inode) recursiveUnmount() {
	for _, v := range me.Children {
		v.recursiveUnmount()
	}
	me.mount = nil
}

func (me *inode) IsDir() bool {
	return me.Children != nil
}

func (me *inode) GetMountDirEntries() (out []DirEntry) {
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	for k, _ := range me.Mounts {
		out = append(out, DirEntry{
		Name: k,
		Mode: S_IFDIR,
		})
	}
	return out
}

const initDirSize = 20

func (me *inode) verify(cur *mountData) {
	if !(me.NodeId == FUSE_ROOT_ID || me.LookupCount > 0 || len(me.Children) > 0 || me.mountPoint != nil) {
		p, _ := me.GetPath()
		panic(fmt.Sprintf("node %v %d should be dead: %v %v", p, me.NodeId, len(me.Children), me.LookupCount))
	}
	if me.mountPoint != nil {
		if me != me.mountPoint.mountPoint {
			panic("mountpoint mismatch")
		}
		cur = me.mountPoint
	}
	if me.mount != cur {
		panic(fmt.Sprintf("me.mount not set correctly %v %v", me.mount, cur))
	}

	for name, m := range me.Mounts {
		if m.mountPoint != me.Children[name] {
			panic(fmt.Sprintf("mountpoint parent mismatch: node:%v name:%v ch:%v",
				me.mountPoint, name, me.Children))
		}
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
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()
	if me.mount == nil {
		// Node from unmounted file system.
		return ".deleted", nil
	}

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
	inodeMap *HandleMap
	rootNode *inode
}

func (me *FileSystemConnector) Statistics() string {
	return fmt.Sprintf("Inodes %20d\n", me.inodeMap.Count())
}

func (me *FileSystemConnector) getOpenedFile(h uint64) *openedFile {
	b := (*openedFile)(unsafe.Pointer(DecodeHandle(h)))
	return b
}

type rawDir interface {
	ReadDir(input *ReadIn) (*DirEntryList, Status)
	Release()
}

func (me *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	me.inodeMap.verify()
	root := me.rootNode
	root.verify(me.rootNode.mountPoint)
}

func (me *FileSystemConnector) newInode(root bool, isDir bool) *inode {
	data := new(inode)
	data.NodeId = me.inodeMap.Register(&data.Handled)
	if root {
		me.rootNode = data
		data.NodeId = FUSE_ROOT_ID
	}
	if isDir {
		data.Children = make(map[string]*inode, initDirSize)
	}

	return data
}

func (me *FileSystemConnector) lookupUpdate(parent *inode, name string, isDir bool, lookupCount int) *inode {
	defer me.verify()

	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()

	data, ok := parent.Children[name]
	if !ok {
		data = me.newInode(false, isDir)
		data.Name = name
		data.setParent(parent)
		data.mount = parent.mount
		data.treeLock = &data.mount.treeLock
	}
	data.LookupCount += lookupCount
	return data
}

func (me *FileSystemConnector) lookupMount(parent *inode, name string, lookupCount int) (path string, mount *mountData, isMount bool) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.Mounts == nil {
		return "", nil, false
	}

	mount, ok := parent.Mounts[name]
	if ok {
		mount.treeLock.Lock()
		defer mount.treeLock.Unlock()
		mount.mountPoint.LookupCount += lookupCount
		return "", mount, true
	}
	return "", nil, false
}

func (me *FileSystemConnector) getInodeData(nodeid uint64) *inode {
	if nodeid == FUSE_ROOT_ID {
		return me.rootNode
	}
	return (*inode)(unsafe.Pointer(DecodeHandle(nodeid)))
}

func (me *FileSystemConnector) forgetUpdate(nodeId uint64, forgetCount int) {
	defer me.verify()

	node := me.getInodeData(nodeId)

	node.LookupCount -= forgetCount
	me.considerDropInode(node)
}

func (me *FileSystemConnector) considerDropInode(n *inode) {
	if n.Parent != nil {
		n.Parent.treeLock.Lock()
		defer n.Parent.treeLock.Unlock()
	}
	if n.Parent == nil || n.Parent.treeLock != n.treeLock {
		n.treeLock.Lock()
		defer n.treeLock.Unlock()
	}

	n.OpenCountMutex.Lock()
	defer n.OpenCountMutex.Unlock()
	dropInode := n.LookupCount <= 0 && len(n.Children) == 0 &&
		n.OpenCount <= 0 && n != me.rootNode
	if dropInode {
		if n.mountPoint != nil {
			me.unsafeUnmountNode(n)
		} else {
			n.setParent(nil)
		}
		if n != me.rootNode {
			me.inodeMap.Forget(n.NodeId)
		}
	}
}

func (me *FileSystemConnector) renameUpdate(oldParent *inode, oldName string, newParent *inode, newName string) {
	defer me.verify()
	oldParent.treeLock.Lock()
	defer oldParent.treeLock.Unlock()

	if oldParent.mount != newParent.mount {
		panic("Cross mount rename")
	}

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

	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()

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
	out.inodeMap = NewHandleMap()

	out.newInode(true, true)
	out.verify()
	return out
}

// Mount() generates a synthetic directory node, and mounts the file
// system there.  If opts is nil, the mount options of the root file
// system are inherited.  The encompassing filesystem should pretend
// the mount point does not exist.  If it does, it will generate an
// inode with the same, which will cause Mount() to return EBUSY.
//
// Return values:
//
// ENOENT: the directory containing the mount point does not exist.
//
// EBUSY: the intended mount point already exists.
func (me *FileSystemConnector) Mount(mountPoint string, fs FileSystem, opts *FileSystemOptions) Status {
	var node *inode
	var parent *inode
	if mountPoint != "/" {
		dirParent, base := filepath.Split(mountPoint)
		parent = me.findInode(dirParent)
		if parent == nil {
			log.Println("Could not find mountpoint parent:", dirParent)
			return ENOENT
		}

		parent.treeLock.Lock()
		defer parent.treeLock.Unlock()
		if parent.mount == nil {
			return ENOENT
		}
		node = parent.Children[base]
		if node != nil {
			return EBUSY
		}

		node = me.newInode(false, true)
		node.Name = base
		node.setParent(parent)
		if opts == nil {
			opts = me.rootNode.mountPoint.options
		}
	} else {
		node = me.rootNode
		if opts == nil {
			opts = NewFileSystemOptions()
		}
	}

	node.mountPoint = newMount(fs)
	node.treeLock = &node.mountPoint.treeLock
	node.mount = node.mountPoint
	node.mountPoint.mountPoint = node
	if parent != nil {
		if parent.Mounts == nil {
			parent.Mounts = make(map[string]*mountData)
		}
		parent.Mounts[node.Name] = node.mountPoint
	}

	node.mountPoint.options = opts

	if me.Debug {
		log.Println("Mount: ", fs, "on dir", mountPoint,
			"parent", parent)
	}
	fs.Mount(me)
	return OK
}

// Unmount() tries to unmount the given path.  Because of kernel-side
// caching, it may takes a few seconds for files to disappear when
// viewed from user-space.
//
// Returns the following error codes:
//
// EINVAL: path does not exist, or is not a mount point.
//
// EBUSY: there are open files, or submounts below this node.
func (me *FileSystemConnector) Unmount(path string) Status {
	node := me.findInode(path)
	if node == nil {
		log.Println("Could not find mountpoint:", path)
		return EINVAL
	}

	parentNode := node.Parent
	if parentNode == nil {
		// attempt to unmount root?
		return EINVAL
	}

	// Must lock parent to update tree structure.
	parentNode.treeLock.Lock()
	defer parentNode.treeLock.Unlock()

	if node.treeLock != parentNode.treeLock {
		node.treeLock.Lock()
		defer node.treeLock.Unlock()
	}
	if node.mountPoint == nil {
		return EINVAL
	}

	if node.mountPoint.openFiles.Count() > 0 {
		return EBUSY
	}

	if !node.canUnmount() {
		return EBUSY
	}

	me.unsafeUnmountNode(node)
	return OK
}

// Assumes node.treeLock and node.Parent.treeLock have been taken.
func (me *FileSystemConnector) unsafeUnmountNode(node *inode) {
	if node == me.rootNode {
		return
	}
	node.recursiveUnmount()
	unmounted := node.mountPoint
	unmounted.mountPoint = nil
	node.mountPoint = nil

	parentNode := node.Parent
	node.Parent = nil
	if parentNode != nil {
		parentNode.Mounts[node.Name] = nil, false
		parentNode.Children[node.Name] = nil, false
	}
	unmounted.fs.Unmount()
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
	node := me.getInodeData(nodeid)
	node.treeLock.RLock()
	defer node.treeLock.RUnlock()

	if fh != 0 {
		opened := me.getOpenedFile(fh)
		m = opened.mountData
		f = opened.file
	}

	path, mount := node.GetPath()

	// If the file was deleted, GetPath() will return nil.
	if mount != nil {
		m = mount
		p = path
	}
	return
}
