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
	*fileSystemMount
	*inode

	// O_CREAT, O_TRUNC, etc.
	OpenFlags uint32

	// FOPEN_KEEP_CACHE and friends.
	FuseFlags uint32

	dir  rawDir
	file File
}

type fileSystemMount struct {
	// If non-nil the file system mounted here.
	fs FileSystem

	// Node that we were mounted on.
	mountInode *inode

	// We could have separate treeLocks per mount; something to
	// consider if we can measure significant contention for
	// multi-mount filesystems.
	options *FileSystemOptions

	// Protects parent/child relations within the mount.
	// treeLock should be acquired before openFilesLock
	treeLock sync.RWMutex

	// Manage filehandles of open files.
	openFiles HandleMap
}

func (me *FileSystemConnector) getOpenedFile(h uint64) *openedFile {
	b := (*openedFile)(unsafe.Pointer(DecodeHandle(h)))
	return b
}

func (me *fileSystemMount) unregisterFileHandle(handle uint64) *openedFile {
	obj := me.openFiles.Forget(handle)
	opened := (*openedFile)(unsafe.Pointer(obj))
	node := opened.inode
	node.OpenFilesMutex.Lock()
	defer node.OpenFilesMutex.Unlock()

	idx := -1
	for i, v := range node.OpenFiles {
		if v == opened {
			idx = i
			break
		}
	}

	l := len(node.OpenFiles)
	node.OpenFiles[idx] = node.OpenFiles[l-1]
	node.OpenFiles = node.OpenFiles[:l-1]

	return opened
}

func (me *fileSystemMount) registerFileHandle(node *inode, dir rawDir, f File, flags uint32) (uint64, *openedFile) {
	node.OpenFilesMutex.Lock()
	defer node.OpenFilesMutex.Unlock()
	b := &openedFile{
		dir:             dir,
		file:            f,
		inode:           node,
		fileSystemMount: me,
		OpenFlags:       flags,
	}

	withFlags, ok := f.(*WithFlags)
	if ok {
		b.FuseFlags = withFlags.Flags
		f = withFlags.File
	}

	node.OpenFiles = append(node.OpenFiles, b)
	handle := me.openFiles.Register(&b.Handled)
	return handle, b
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
	NodeId uint64

	// Number of open files and its protection.
	OpenFilesMutex sync.Mutex
	OpenFiles      []*openedFile

	// treeLock is a pointer to me.mount.treeLock; we need store
	// this mutex separately, since unmount may set me.mount = nil
	// during Unmount().  Constant during lifetime.
	//
	// If multiple treeLocks must be acquired, the treeLocks
	// closer to the root must be acquired first.
	treeLock *sync.RWMutex

	// All data below is protected by treeLock.
	Name        string
	Parent      *inode
	Children    map[string]*inode
	Mounts      map[string]*fileSystemMount
	LookupCount int

	// Non-nil if this is a mountpoint.
	mountPoint *fileSystemMount

	// The file system to which this node belongs.  Is constant
	// during the lifetime, except upon Unmount() when it is set
	// to nil.
	mount *fileSystemMount
}

// Can only be called on untouched inodes.
func (me *inode) mountFs(fs FileSystem, opts *FileSystemOptions) {
	me.mountPoint = &fileSystemMount{
		fs:         fs,
		openFiles:  NewHandleMap(true),
		mountInode: me,
		options:    opts,
	}
	me.mount = me.mountPoint
	me.treeLock = &me.mountPoint.treeLock
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

	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()
	return len(me.OpenFiles) == 0
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

func (me *inode) verify(cur *fileSystemMount) {
	if !(me.NodeId == FUSE_ROOT_ID || me.LookupCount > 0 || len(me.Children) > 0 || me.mountPoint != nil) {
		p, _ := me.GetPath()
		panic(fmt.Sprintf("node %v %d should be dead: %v %v", p, me.NodeId, len(me.Children), me.LookupCount))
	}
	if me.mountPoint != nil {
		if me != me.mountPoint.mountInode {
			panic("mountpoint mismatch")
		}
		cur = me.mountPoint
	}
	if me.mount != cur {
		panic(fmt.Sprintf("me.mount not set correctly %v %v", me.mount, cur))
	}

	for name, m := range me.Mounts {
		if m.mountInode != me.Children[name] {
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
// filesystem unmounted.  This will take the treeLock for the mount,
// so it can not be used in internal methods.
func (me *inode) GetPath() (path string, mount *fileSystemMount) {
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

	fsInit   RawFsInit
	inodeMap HandleMap
	rootNode *inode
}

func (me *FileSystemConnector) Init(fsInit *RawFsInit) {
	me.fsInit = *fsInit
}

func (me *FileSystemConnector) Statistics() string {
	return fmt.Sprintf("Inodes %20d\n", me.inodeMap.Count())
}

func (me *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	root := me.rootNode
	root.verify(me.rootNode.mountPoint)
}

func (me *FileSystemConnector) newInode(isDir bool) *inode {
	data := new(inode)
	data.NodeId = me.inodeMap.Register(&data.Handled)
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
		data = me.newInode(isDir)
		data.Name = name
		data.setParent(parent)
		data.mount = parent.mount
		data.treeLock = &data.mount.treeLock
	}
	data.LookupCount += lookupCount
	return data
}

func (me *FileSystemConnector) lookupMount(parent *inode, name string, lookupCount int) (path string, mount *fileSystemMount, isMount bool) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.Mounts == nil {
		return "", nil, false
	}

	mount, ok := parent.Mounts[name]
	if ok {
		mount.treeLock.Lock()
		defer mount.treeLock.Unlock()
		mount.mountInode.LookupCount += lookupCount
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

	n.OpenFilesMutex.Lock()
	defer n.OpenFilesMutex.Unlock()
	dropInode := n.LookupCount <= 0 && len(n.Children) == 0 &&
		len(n.OpenFiles) <= 0 && n != me.rootNode && n.mountPoint == nil
	if dropInode {
		n.setParent(nil)
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
func (me *FileSystemConnector) findLastKnownInode(fullPath string) (*inode, []string) {
	if fullPath == "" {
		return me.rootNode, nil
	}

	fullPath = strings.TrimLeft(filepath.Clean(fullPath), "/")
	comps := strings.Split(fullPath, "/")

	node := me.rootNode
	for i, component := range comps {
		if len(component) == 0 {
			continue
		}

		if node.mountPoint != nil {
			node.mountPoint.treeLock.RLock()
			defer node.mountPoint.treeLock.RUnlock()
		}

		next := node.Children[component]
		if next == nil {
			return node, comps[i:]
		}
		node = next
	}

	return node, nil
}

func (me *FileSystemConnector) findInode(fullPath string) *inode {
	n, rest := me.findLastKnownInode(fullPath)
	if len(rest) > 0 {
		return nil
	}
	return n
}

////////////////////////////////////////////////////////////////

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
//
// TODO - would be useful to expose an interface to put all of the
// mount management in FileSystemConnector, so AutoUnionFs and
// MultiZipFs don't have to do it separately, with the risk of
// inconsistencies.
func (me *FileSystemConnector) Mount(mountPoint string, fs FileSystem, opts *FileSystemOptions) Status {
	if mountPoint == "/" || mountPoint == "" {
		me.mountRoot(fs, opts)
		return OK
	}

	dirParent, base := filepath.Split(mountPoint)
	parent := me.findInode(dirParent)
	if parent == nil {
		log.Println("Could not find mountpoint parent:", dirParent)
		return ENOENT
	}

	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()
	if parent.mount == nil {
		return ENOENT
	}
	node := parent.Children[base]
	if node != nil {
		return EBUSY
	}

	node = me.newInode(true)
	node.Name = base
	node.setParent(parent)
	if opts == nil {
		opts = me.rootNode.mountPoint.options
	}

	node.mountFs(fs, opts)
	if parent.Mounts == nil {
		parent.Mounts = make(map[string]*fileSystemMount)
	}
	parent.Mounts[node.Name] = node.mountPoint
	if me.Debug {
		log.Println("Mount: ", fs, "on dir", mountPoint,
			"parent", parent)
	}
	fs.Mount(me)
	me.verify()
	return OK
}

func (me *FileSystemConnector) mountRoot(fs FileSystem, opts *FileSystemOptions) {
	me.rootNode.mountFs(fs, opts)
	fs.Mount(me)
	me.verify()
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
	notifyMessage := NotifyInvalInodeOut{
		Ino: node.NodeId,
	}
	me.fsInit.InodeNotify(&notifyMessage)

	return OK
}

// Assumes node.treeLock and node.Parent.treeLock have been taken.
func (me *FileSystemConnector) unsafeUnmountNode(node *inode) {
	if node == me.rootNode {
		return
	}
	node.recursiveUnmount()
	unmounted := node.mountPoint
	unmounted.mountInode = nil
	node.mountPoint = nil

	parentNode := node.Parent
	node.Parent = nil
	if parentNode != nil {
		parentNode.Mounts[node.Name] = nil, false
		parentNode.Children[node.Name] = nil, false
	}
	unmounted.fs.Unmount()
}

// Returns an openedFile for the gived inode.
func (me *FileSystemConnector) getOpenFileData(nodeid uint64, fh uint64) (opened *openedFile, m *fileSystemMount, p string, node *inode) {
	node = me.getInodeData(nodeid)
	if fh != 0 {
		opened = me.getOpenedFile(fh)
	}

	path, mount := node.GetPath()
	if me.Debug {
		log.Printf("Node %v = '%s'", nodeid, path)
	}

	// If the file was deleted, GetPath() will return nil.
	if mount != nil {
		m = mount
		p = path
	}
	if opened == nil {
		node.OpenFilesMutex.Lock()
		defer node.OpenFilesMutex.Unlock()

		for _, f := range node.OpenFiles {
			if f.OpenFlags & O_ANYWRITE != 0 || opened == nil {
				opened = f
			}
		}
	}
	return
}

func (me *FileSystemConnector) FileNotify(path string, off int64, length int64) Status {
	node := me.findInode(path)
	if node == nil {
		return ENOENT
	}

	out := NotifyInvalInodeOut{
		Length: length,
		Off:    off,
		Ino:    node.NodeId,
	}
	return me.fsInit.InodeNotify(&out)
}

func (me *FileSystemConnector) EntryNotify(dir string, name string) Status {
	node := me.findInode(dir)
	if node == nil {
		return ENOENT
	}

	return me.fsInit.EntryNotify(node.NodeId, name)
}

func (me *FileSystemConnector) Notify(path string) Status {
	node, rest := me.findLastKnownInode(path)
	if len(rest) > 0 {
		return me.fsInit.EntryNotify(node.NodeId, rest[0])
	}
	out := NotifyInvalInodeOut{
		Ino: node.NodeId,
	}
	return me.fsInit.InodeNotify(&out)
}
