package fuse

// This file contains the internal logic of the
// FileSystemConnector. The functions for satisfying the raw interface
// are in fsops.go

import (
	"log"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

// Tests should set to true.
var paranoia = false

// FilesystemConnector is a raw FUSE filesystem that manages
// in-process mounts and inodes.  Its job is twofold:
//
// * It translates between the raw kernel interface (padded structs of
// int32 and int64) and the more abstract Go-ish NodeFileSystem
// interface.
//
// * It manages mounting and unmounting of NodeFileSystems into the
// directory hierarchy
//
// To achieve this, the connector only needs a pointer to the root
// node.
type FileSystemConnector struct {
	DefaultRawFileSystem

	Debug bool

	// Callbacks for talking back to the kernel.
	fsInit RawFsInit

	// Translate between uint64 handles and *Inode.
	inodeMap HandleMap

	// The root of the FUSE file system.
	rootNode *Inode

	// Used as the generation inodes.
	generation uint64
}

func NewFileSystemOptions() *FileSystemOptions {
	return &FileSystemOptions{
		NegativeTimeout: 0,
		AttrTimeout:     time.Second,
		EntryTimeout:    time.Second,
		Owner:           CurrentOwner(),
	}
}

func NewFileSystemConnector(nodeFs NodeFileSystem, opts *FileSystemOptions) (c *FileSystemConnector) {
	c = new(FileSystemConnector)
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	c.inodeMap = NewHandleMap(opts.PortableInodes)
	c.rootNode = newInode(true, nodeFs.Root())

	// Make sure we don't reuse generation numbers.
	c.generation = uint64(time.Now().UnixNano())

	c.verify()
	c.MountRoot(nodeFs, opts)

	// FUSE does not issue a LOOKUP for 1 (obviously), but it does
	// issue a forget.  This lookupUpdate is to make the counts match.
	c.lookupUpdate(c.rootNode)

	return c
}

func (c *FileSystemConnector) nextGeneration() uint64 {
	return atomic.AddUint64(&c.generation, 1)
}

// This verifies invariants of the data structure.  This routine
// acquires tree locks as it walks the inode tree.
func (c *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	root := c.rootNode
	root.verify(c.rootNode.mountPoint)
}

// Generate EntryOut and increase the lookup count for an inode.
func (c *FileSystemConnector) childLookup(out *raw.EntryOut, fsi FsNode) {
	n := fsi.Inode()
	fsi.GetAttr((*Attr)(&out.Attr), nil, nil)
	n.mount.fillEntry(out)
	out.Ino = c.lookupUpdate(n)
	out.NodeId = out.Ino
	if out.Nlink == 0 {
		// With Nlink == 0, newer kernels will refuse link
		// operations.
		out.Nlink = 1
	}
}

func (c *FileSystemConnector) toInode(nodeid uint64) *Inode {
	if nodeid == raw.FUSE_ROOT_ID {
		return c.rootNode
	}
	i := (*Inode)(unsafe.Pointer(c.inodeMap.Decode(nodeid)))
	return i
}

// Must run outside treeLock.  Returns the nodeId.
func (c *FileSystemConnector) lookupUpdate(node *Inode) (id uint64) {
	id = c.inodeMap.Register(&node.handled)
	c.verify()
	return id
}

// Must run outside treeLock.
func (c *FileSystemConnector) forgetUpdate(nodeID uint64, forgetCount int) {
	if nodeID == raw.FUSE_ROOT_ID {
		// We never got a lookup for root, so don't try to forget root.
		return
	}

	if forgotten, handled := c.inodeMap.Forget(nodeID, forgetCount); forgotten {
		node := (*Inode)(unsafe.Pointer(handled))
		node.mount.treeLock.Lock()
		c.recursiveConsiderDropInode(node)
		node.mount.treeLock.Unlock()
	}
	// TODO - try to drop children even forget was not successful.
	c.verify()
}

// InodeCount returns the number of inodes registered with the kernel.
func (c *FileSystemConnector) InodeHandleCount() int {
	return c.inodeMap.Count()
}

// Must hold treeLock.

func (c *FileSystemConnector) recursiveConsiderDropInode(n *Inode) (drop bool) {
	delChildren := []string{}
	for k, v := range n.children {
		// Only consider children from the same mount, or
		// already unmounted mountpoints.
		if v.mountPoint == nil && c.recursiveConsiderDropInode(v) {
			delChildren = append(delChildren, k)
		}
	}
	for _, k := range delChildren {
		ch := n.rmChild(k)
		if ch == nil {
			log.Panicf("trying to del child %q, but not present", k)
		}
		ch.fsInode.OnForget()
	}

	if len(n.children) > 0 || !n.FsNode().Deletable() {
		return false
	}
	if n == c.rootNode || n.mountPoint != nil {
		return false
	}

	n.openFilesMutex.Lock()
	ok := len(n.openFiles) == 0
	n.openFilesMutex.Unlock()

	return ok
}

// Finds a node within the currently known inodes, returns the last
// known node and the remaining unknown path components.  If parent is
// nil, start from FUSE mountpoint.
func (c *FileSystemConnector) Node(parent *Inode, fullPath string) (*Inode, []string) {
	if parent == nil {
		parent = c.rootNode
	}
	if fullPath == "" {
		return parent, nil
	}

	sep := string(filepath.Separator)
	fullPath = strings.TrimLeft(filepath.Clean(fullPath), sep)
	comps := strings.Split(fullPath, sep)

	node := parent
	if node.mountPoint == nil {
		node.mount.treeLock.RLock()
		defer node.mount.treeLock.RUnlock()
	}

	for i, component := range comps {
		if len(component) == 0 {
			continue
		}

		if node.mountPoint != nil {
			node.mount.treeLock.RLock()
			defer node.mount.treeLock.RUnlock()
		}

		next := node.children[component]
		if next == nil {
			return node, comps[i:]
		}
		node = next
	}

	return node, nil
}

func (c *FileSystemConnector) LookupNode(parent *Inode, path string) *Inode {
	if path == "" {
		return parent
	}
	components := strings.Split(path, "/")
	for _, r := range components {
		var a Attr
		child, _ := c.internalLookup(&a, parent, r, nil)
		if child == nil {
			return nil
		}

		parent = child
	}

	return parent
}

////////////////////////////////////////////////////////////////

func (c *FileSystemConnector) MountRoot(nodeFs NodeFileSystem, opts *FileSystemOptions) {
	c.rootNode.mountFs(nodeFs, opts)
	c.rootNode.mount.connector = c
	nodeFs.OnMount(c)
	c.verify()
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
func (c *FileSystemConnector) Mount(parent *Inode, name string, nodeFs NodeFileSystem, opts *FileSystemOptions) Status {
	defer c.verify()
	parent.mount.treeLock.Lock()
	defer parent.mount.treeLock.Unlock()
	node := parent.children[name]
	if node != nil {
		return EBUSY
	}

	node = newInode(true, nodeFs.Root())
	if opts == nil {
		opts = c.rootNode.mountPoint.options
	}

	node.mountFs(nodeFs, opts)
	node.mount.connector = c
	parent.addChild(name, node)

	node.mountPoint.parentInode = parent
	if c.Debug {
		log.Println("Mount: ", nodeFs, "on subdir", name,
			"parent", c.inodeMap.Handle(&parent.handled))
	}
	nodeFs.OnMount(c)
	return OK
}

// Unmount() tries to unmount the given inode.
//
// Returns the following error codes:
//
// EINVAL: path does not exist, or is not a mount point.
//
// EBUSY: there are open files, or submounts below this node.
func (c *FileSystemConnector) Unmount(node *Inode) Status {
	// TODO - racy.
	if node.mountPoint == nil {
		log.Println("not a mountpoint:", c.inodeMap.Handle(&node.handled))
		return EINVAL
	}

	// Must lock parent to update tree structure.
	parentNode := node.mountPoint.parentInode
	parentNode.mount.treeLock.Lock()
	defer parentNode.mount.treeLock.Unlock()

	mount := node.mountPoint
	name := node.mountPoint.mountName()
	if mount.openFiles.Count() > 0 {
		return EBUSY
	}

	node.mount.treeLock.Lock()
	defer node.mount.treeLock.Unlock()

	if mount.mountInode != node {
		log.Panicf("got two different mount inodes %v vs %v",
			c.inodeMap.Handle(&mount.mountInode.handled),
			c.inodeMap.Handle(&node.handled))
	}

	if !node.canUnmount() {
		return EBUSY
	}

	mount.mountInode = nil
	// TODO - racy.
	node.mountPoint = nil

	delete(parentNode.children, name)
	mount.fs.OnUnmount()

	parentId := c.inodeMap.Handle(&parentNode.handled)
	if parentNode == c.rootNode {
		// TODO - test coverage. Currently covered by zipfs/multizip_test.go
		parentId = raw.FUSE_ROOT_ID
	}

	c.fsInit.DeleteNotify(parentId, c.inodeMap.Handle(&node.handled), name)
	return OK
}

func (c *FileSystemConnector) FileNotify(node *Inode, off int64, length int64) Status {
	var nId uint64
	if node == c.rootNode {
		nId = raw.FUSE_ROOT_ID
	} else {
		nId = c.inodeMap.Handle(&node.handled)
	}

	if nId == 0 {
		return OK
	}
	out := raw.NotifyInvalInodeOut{
		Length: length,
		Off:    off,
		Ino:    nId,
	}
	return c.fsInit.InodeNotify(&out)
}

func (c *FileSystemConnector) EntryNotify(node *Inode, name string) Status {
	var nId uint64
	if node == c.rootNode {
		nId = raw.FUSE_ROOT_ID
	} else {
		nId = c.inodeMap.Handle(&node.handled)
	}

	if nId == 0 {
		return OK
	}
	return c.fsInit.EntryNotify(nId, name)
}

func (c *FileSystemConnector) DeleteNotify(dir *Inode, child *Inode, name string) Status {
	var nId uint64

	if dir == c.rootNode {
		nId = raw.FUSE_ROOT_ID
	} else {
		nId = c.inodeMap.Handle(&dir.handled)
	}

	if nId == 0 {
		return OK
	}

	chId := c.inodeMap.Handle(&child.handled)

	return c.fsInit.DeleteNotify(nId, chId, name)
}
