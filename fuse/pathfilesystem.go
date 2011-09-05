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
	"os"
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

	options *FileSystemOptions

	// Protects parent/child relations within the mount.
	// treeLock should be acquired before openFilesLock
	treeLock sync.RWMutex

	// Manage filehandles of open files.
	openFiles HandleMap
}

func (me *fileSystemMount) fileInfoToEntry(fi *os.FileInfo, out *EntryOut) {
	SplitNs(me.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	if !fi.IsDirectory() {
		fi.Nlink = 1
	}

	CopyFileInfo(fi, &out.Attr)
	me.setOwner(&out.Attr)
}

	
func (me *fileSystemMount) fileInfoToAttr(fi *os.FileInfo, out *AttrOut) {
	CopyFileInfo(fi, &out.Attr)
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	me.setOwner(&out.Attr)
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

// The inode reflects the kernel's idea of the inode.
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
	fsInode *fsInode
	
	Children    map[string]*inode

	// Contains directories that function as mounts. The entries
	// are duplicated in Children.
	Mounts      map[string]*fileSystemMount
	LookupCount int

	// Non-nil if this is a mountpoint.
	mountPoint *fileSystemMount

	// The file system to which this node belongs.  Is constant
	// during the lifetime, except upon Unmount() when it is set
	// to nil.
	mount *fileSystemMount
}

// Must be called with treeLock for the mount held.
func (me *inode) addChild(name string, child *inode) {
	if paranoia {
		ch := me.Children[name]
		if ch != nil {
			panic(fmt.Sprintf("Already have an inode with same name: %v: %v", name, ch))
		}
	}

	me.Children[name] = child
	me.fsInode.addChild(name, child.fsInode)
}

// Must be called with treeLock for the mount held.
func (me *inode) rmChild(name string) (ch *inode) {
	ch = me.Children[name]
	if ch != nil {
		me.Children[name] = nil, false
		me.fsInode.rmChild(name, ch.fsInode)
	}
	return ch
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

func (me *inode) getMountDirEntries() (out []DirEntry) {
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

// Returns any open file, preferably a r/w one.
func (me *inode) getAnyFile() (file File) {
	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()
	
	for _, f := range me.OpenFiles {
		if file == nil || f.OpenFlags & O_ANYWRITE != 0 {
			file = f.file
		}
	}
	return file
}

// Returns an open writable file for the given inode.
func (me *inode) getWritableFiles() (files []File) {
	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()

	for _, f := range me.OpenFiles {
		if f.OpenFlags & O_ANYWRITE != 0 {
			files = append(files, f.file)
		}
	}
	return files
}

const initDirSize = 20

func (me *inode) verify(cur *fileSystemMount) {
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
	
	for _, ch := range me.Children {
		if ch == nil {
			panic("Found nil child.")
		}
		ch.verify(cur)
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
	data.fsInode = new(fsInode)
	data.fsInode.inode = data
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
		parent.addChild(name, data)
		data.mount = parent.mount
		data.treeLock = &data.mount.treeLock
	}
	data.LookupCount += lookupCount
	return data
}

func (me *FileSystemConnector) lookupMount(parent *inode, name string, lookupCount int) (mount *fileSystemMount) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.Mounts == nil {
		return nil
	}

	mount, ok := parent.Mounts[name]
	if ok {
		mount.treeLock.Lock()
		defer mount.treeLock.Unlock()
		mount.mountInode.LookupCount += lookupCount
		return mount
	}
	return nil
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

	node.treeLock.Lock()
	defer node.treeLock.Unlock()

	node.LookupCount -= forgetCount
	me.considerDropInode(node)
}

func (me *FileSystemConnector) considerDropInode(n *inode) (drop bool) {
	delChildren := []string{}
	for k, v := range n.Children {
		if v.mountPoint == nil && me.considerDropInode(v) {
			delChildren = append(delChildren, k)
		}
	}
	for _, k := range delChildren {
		ch := n.rmChild(k)
		if ch == nil {
			panic(fmt.Sprintf("trying to del child %q, but not present", k))
		}
		me.inodeMap.Forget(ch.NodeId)
	}

	if len(n.Children) > 0 || n.LookupCount > 0 {
		return false
	}
	if n == me.rootNode || n.mountPoint != nil {
		return false
	}
	
	n.OpenFilesMutex.Lock()
	defer n.OpenFilesMutex.Unlock()
	return len(n.OpenFiles) == 0 
}

func (me *FileSystemConnector) renameUpdate(oldParent *inode, oldName string, newParent *inode, newName string) {
	defer me.verify()
	oldParent.treeLock.Lock()
	defer oldParent.treeLock.Unlock()

	if oldParent.mount != newParent.mount {
		panic("Cross mount rename")
	}
	
	node := oldParent.rmChild(oldName)
	if node == nil {
		panic("Source of rename does not exist")
	}
	newParent.rmChild(newName)
	newParent.addChild(newName, node)
}

func (me *FileSystemConnector) unlinkUpdate(parent *inode, name string) {
	defer me.verify()
	
	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()

	parent.rmChild(name)
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
	if opts == nil {
		opts = me.rootNode.mountPoint.options
	}

	node.mountFs(fs, opts)
	parent.addChild(base, node)

	if parent.Mounts == nil {
		parent.Mounts = make(map[string]*fileSystemMount)
	}
	parent.Mounts[base] = node.mountPoint
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

// Unmount() tries to unmount the given path.  
//
// Returns the following error codes:
//
// EINVAL: path does not exist, or is not a mount point.
//
// EBUSY: there are open files, or submounts below this node.
func (me *FileSystemConnector) Unmount(path string) Status {
	dir, name := filepath.Split(path)
	parentNode := me.findInode(dir)
	if parentNode == nil {
		log.Println("Could not find parent of mountpoint:", path)
		return EINVAL
	}

	// Must lock parent to update tree structure.
	parentNode.treeLock.Lock()
	defer parentNode.treeLock.Unlock()

	mount := parentNode.Mounts[name]
	if mount == nil {
		return EINVAL
	}

	if mount.openFiles.Count() > 0 {
		return EBUSY
	}

	mountInode := mount.mountInode
	if !mountInode.canUnmount() {
		return EBUSY
	}

	mountInode.recursiveUnmount()
	mount.mountInode = nil
	mountInode.mountPoint = nil

	parentNode.Mounts[name] = nil, false
	parentNode.Children[name] = nil, false
	mount.fs.Unmount()

	me.fsInit.EntryNotify(parentNode.NodeId, name)

	return OK
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
