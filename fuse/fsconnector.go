package fuse

// This file contains the internal logic of the
// FileSystemConnector. The functions for satisfying the raw interface are in
// fsops.go

import (
	"fmt"
	"log"
	"os"
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
	rootNode *Inode
}

func NewFileSystemConnector(nodeFs NodeFileSystem, opts *FileSystemOptions) (me *FileSystemConnector) {
	me = new(FileSystemConnector)
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	me.inodeMap = NewHandleMap(!opts.SkipCheckHandles)
	me.rootNode = me.newInode(true)
	me.rootNode.nodeId = FUSE_ROOT_ID

	// FUSE does not issue a LOOKUP for 1 (obviously), but it does
	// issue a forget.  This lookupCount is to make the counts match.
	me.rootNode.addLookupCount(1)
	me.verify()
	me.MountRoot(nodeFs, opts)
	return me
}

func (me *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	root := me.rootNode
	root.verify(me.rootNode.mountPoint)
}

func (me *FileSystemConnector) newInode(isDir bool) *Inode {
	data := new(Inode)
	data.nodeId = me.inodeMap.Register(&data.handled, data)
	data.connector = me
	if isDir {
		data.children = make(map[string]*Inode, initDirSize)
	}

	return data
}

// createChild() creates a child for given as FsNode as child of 'parent'.  The
// resulting inode will have its lookupCount incremented.
func (me *FileSystemConnector) createChild(parent *Inode, name string, fi *os.FileInfo, fsi FsNode) (out *EntryOut, child *Inode) {
	if fsi.Inode() == nil {
		child = parent.CreateChild(name, fi.IsDirectory(), fsi)
	} else {
		parent.treeLock.Lock()
		defer parent.treeLock.Unlock()
		child = fsi.Inode()
		child.addLookupCount(1)
	}

	out = parent.mount.fileInfoToEntry(fi)
	out.Ino = child.nodeId
	out.NodeId = child.nodeId
	return out, child
}

func (me *FileSystemConnector) lookupUpdate(parent *Inode, name string, isDir bool, lookupCount int) *Inode {
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
	data.addLookupCount(lookupCount)
	return data
}

func (me *FileSystemConnector) findMount(parent *Inode, name string) (mount *fileSystemMount) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.mounts == nil {
		return nil
	}

	return parent.mounts[name]
}

func (me *FileSystemConnector) getInodeData(nodeid uint64) *Inode {
	if nodeid == FUSE_ROOT_ID {
		return me.rootNode
	}
	i := (*Inode)(unsafe.Pointer(DecodeHandle(nodeid)))
	return i
}

func (me *FileSystemConnector) forgetUpdate(nodeId uint64, forgetCount int) {
	defer me.verify()

	node := me.getInodeData(nodeId)

	node.treeLock.Lock()
	defer node.treeLock.Unlock()

	node.addLookupCount(-forgetCount)
	me.considerDropInode(node)
}

func (me *FileSystemConnector) considerDropInode(n *Inode) (drop bool) {
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
		ch.fsInode.OnForget()
		me.inodeMap.Forget(ch.nodeId)
	}

	if len(n.children) > 0 || n.Live() {
		return false
	}
	if n == me.rootNode || n.mountPoint != nil {
		return false
	}

	n.openFilesMutex.Lock()
	defer n.openFilesMutex.Unlock()
	return len(n.openFiles) == 0
}

func (me *FileSystemConnector) renameUpdate(oldParent *Inode, oldName string, newParent *Inode, newName string) {
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

func (me *FileSystemConnector) unlinkUpdate(parent *Inode, name string) {
	defer me.verify()

	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()

	parent.rmChild(name)
}

// Walk the file system starting from the root. Will return nil if
// node not found.
func (me *FileSystemConnector) findLastKnownInode(fullPath string) (*Inode, []string) {
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

func (me *FileSystemConnector) findInode(fullPath string) *Inode {
	n, rest := me.findLastKnownInode(fullPath)
	if len(rest) > 0 {
		return nil
	}
	return n
}

////////////////////////////////////////////////////////////////

func (me *FileSystemConnector) MountRoot(nodeFs NodeFileSystem, opts *FileSystemOptions) {
	me.rootNode.mountFs(nodeFs, opts)
	nodeFs.OnMount(me)
	me.verify()
}

// Mount() generates a synthetic directory node, and mounts the file
// system there.  If opts is nil, the mount options of the root file
// system are inherited.  The encompassing filesystem should pretend
// the mount point does not exist.  If it does, it will generate an
// Inode with the same, which will cause Mount() to return EBUSY.
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
func (me *FileSystemConnector) Mount(parent *Inode, name string, nodeFs NodeFileSystem, opts *FileSystemOptions) Status {
	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()
	node := parent.children[name]
	if node != nil {
		return EBUSY
	}

	node = me.newInode(true)
	if opts == nil {
		opts = me.rootNode.mountPoint.options
	}

	node.mountFs(nodeFs, opts)
	parent.addChild(name, node)

	if parent.mounts == nil {
		parent.mounts = make(map[string]*fileSystemMount)
	}
	parent.mounts[name] = node.mountPoint
	node.mountPoint.parentInode = parent
	if me.Debug {
		log.Println("Mount: ", nodeFs, "on subdir", name,
			"parent", parent.nodeId)
	}
	me.verify()
	nodeFs.OnMount(me)
	return OK
}

// Unmount() tries to unmount the given inode.
//
// Returns the following error codes:
//
// EINVAL: path does not exist, or is not a mount point.
//
// EBUSY: there are open files, or submounts below this node.
func (me *FileSystemConnector) Unmount(node *Inode) Status {
	if node.mountPoint == nil {
		log.Println("not a mountpoint:", node.nodeId)
		return EINVAL
	}

	// Must lock parent to update tree structure.
	parentNode := node.mountPoint.parentInode
	parentNode.treeLock.Lock()
	defer parentNode.treeLock.Unlock()

	mount := node.mountPoint
	name := node.mountPoint.mountName()
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
	mount.fs.OnUnmount()

	me.fsInit.EntryNotify(parentNode.nodeId, name)

	return OK
}

func (me *FileSystemConnector) FileNotify(node *Inode, off int64, length int64) Status {
	out := NotifyInvalInodeOut{
		Length: length,
		Off:    off,
		Ino:    node.nodeId,
	}
	return me.fsInit.InodeNotify(&out)
}

func (me *FileSystemConnector) EntryNotify(dir *Inode, name string) Status {
	return me.fsInit.EntryNotify(dir.nodeId, name)
}
