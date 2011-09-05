package fuse

// This file contains the internal logic of the
// FileSystemConnector. The functions for satisfying the raw interface are in
// fsops.go

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"unsafe"
)

// Tests should set to true.
var paranoia = false

func NewFileSystemOptions() *FileSystemOptions {
	return &FileSystemOptions{
		NegativeTimeout: 0.0,
		AttrTimeout:     1.0,
		EntryTimeout:    1.0,
		Owner:           CurrentOwner(),
	}
}

// FilesystemConnector is a raw FUSE filesystem that manages in-process mounts and inodes.
type FileSystemConnector struct {
	DefaultRawFileSystem

	Debug bool

	fsInit   RawFsInit
	inodeMap HandleMap
	rootNode *inode
}

func NewFileSystemConnector(fs FileSystem, opts *FileSystemOptions) (me *FileSystemConnector) {
	me = new(FileSystemConnector)
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	me.inodeMap = NewHandleMap(!opts.SkipCheckHandles)
	me.rootNode = me.newInode(true)
	me.rootNode.nodeId = FUSE_ROOT_ID
	me.verify()
	me.mountRoot(fs, opts)
	return me
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
	data.nodeId = me.inodeMap.Register(&data.handled)
	data.fsInode = new(fsInode)
	data.fsInode.inode = data
	if isDir {
		data.children = make(map[string]*inode, initDirSize)
	}

	return data
}

func (me *FileSystemConnector) lookupUpdate(parent *inode, name string, isDir bool, lookupCount int) *inode {
	defer me.verify()

	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()

	data, ok := parent.children[name]
	if !ok {
		data = me.newInode(isDir)
		parent.addChild(name, data)
		data.mount = parent.mount
		data.treeLock = &data.mount.treeLock
	}
	data.lookupCount += lookupCount
	return data
}

func (me *FileSystemConnector) lookupMount(parent *inode, name string, lookupCount int) (mount *fileSystemMount) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.mounts == nil {
		return nil
	}

	mount, ok := parent.mounts[name]
	if ok {
		mount.treeLock.Lock()
		defer mount.treeLock.Unlock()
		mount.mountInode.lookupCount += lookupCount
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

	node.lookupCount -= forgetCount
	me.considerDropInode(node)
}

func (me *FileSystemConnector) considerDropInode(n *inode) (drop bool) {
	delChildren := []string{}
	for k, v := range n.children {
		if v.mountPoint == nil && me.considerDropInode(v) {
			delChildren = append(delChildren, k)
		}
	}
	for _, k := range delChildren {
		ch := n.rmChild(k)
		if ch == nil {
			panic(fmt.Sprintf("trying to del child %q, but not present", k))
		}
		me.inodeMap.Forget(ch.nodeId)
	}

	if len(n.children) > 0 || n.lookupCount > 0 {
		return false
	}
	if n == me.rootNode || n.mountPoint != nil {
		return false
	}
	
	n.openFilesMutex.Lock()
	defer n.openFilesMutex.Unlock()
	return len(n.openFiles) == 0 
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

		next := node.children[component]
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
	node := parent.children[base]
	if node != nil {
		return EBUSY
	}

	node = me.newInode(true)
	if opts == nil {
		opts = me.rootNode.mountPoint.options
	}

	node.mountFs(fs, opts)
	parent.addChild(base, node)

	if parent.mounts == nil {
		parent.mounts = make(map[string]*fileSystemMount)
	}
	parent.mounts[base] = node.mountPoint
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

	mount := parentNode.mounts[name]
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

	mount.mountInode = nil
	mountInode.mountPoint = nil

	parentNode.mounts[name] = nil, false
	parentNode.children[name] = nil, false
	mount.fs.Unmount()

	me.fsInit.EntryNotify(parentNode.nodeId, name)

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
		Ino:    node.nodeId,
	}
	return me.fsInit.InodeNotify(&out)
}

func (me *FileSystemConnector) EntryNotify(dir string, name string) Status {
	node := me.findInode(dir)
	if node == nil {
		return ENOENT
	}

	return me.fsInit.EntryNotify(node.nodeId, name)
}

func (me *FileSystemConnector) Notify(path string) Status {
	node, rest := me.findLastKnownInode(path)
	if len(rest) > 0 {
		return me.fsInit.EntryNotify(node.nodeId, rest[0])
	}
	out := NotifyInvalInodeOut{
		Ino: node.nodeId,
	}
	return me.fsInit.InodeNotify(&out)
}
