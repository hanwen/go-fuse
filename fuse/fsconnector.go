package fuse

// This file contains the internal logic of the
// FileSystemConnector. The functions for satisfying the raw interface
// are in fsops.go

import (
	"log"
	"path/filepath"
	"strings"
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

	c.verify()
	c.MountRoot(nodeFs, opts)

	// FUSE does not issue a LOOKUP for 1 (obviously), but it does
	// issue a forget.  This lookupUpdate is to make the counts match.
	c.lookupUpdate(c.rootNode)
	return c
}

func (c *FileSystemConnector) verify() {
	if !paranoia {
		return
	}
	root := c.rootNode
	root.verify(c.rootNode.mountPoint)
}

// Generate EntryOut and increase the lookup count for an inode.
func (c *FileSystemConnector) childLookup(fi *raw.Attr, fsi FsNode) (out *raw.EntryOut) {
	n := fsi.Inode()
	out = n.mount.attrToEntry(fi)
	out.Ino = c.lookupUpdate(n)
	out.NodeId = out.Ino
	if out.Nlink == 0 {
		// With Nlink == 0, newer kernels will refuse link
		// operations.
		out.Nlink = 1
	}
	return out
}

func (c *FileSystemConnector) findMount(parent *Inode, name string) (mount *fileSystemMount) {
	parent.treeLock.RLock()
	defer parent.treeLock.RUnlock()
	if parent.mounts == nil {
		return nil
	}

	return parent.mounts[name]
}

func (c *FileSystemConnector) toInode(nodeid uint64) *Inode {
	if nodeid == raw.FUSE_ROOT_ID {
		return c.rootNode
	}
	i := (*Inode)(unsafe.Pointer(c.inodeMap.Decode(nodeid)))
	return i
}

// Must run outside treeLock.  Returns the nodeId.
func (c *FileSystemConnector) lookupUpdate(node *Inode) uint64 {
	node.treeLock.Lock()
	if node.lookupCount == 0 {
		node.nodeId = c.inodeMap.Register(&node.handled, node)
	}
	node.lookupCount += 1
	node.treeLock.Unlock()
	
	return node.nodeId
}

// Must run outside treeLock.
func (c *FileSystemConnector) forgetUpdate(node *Inode, forgetCount int) {
	defer c.verify()

	node.treeLock.Lock()
	node.lookupCount -= forgetCount
	if node.lookupCount == 0 {
		c.inodeMap.Forget(node.nodeId)
		node.nodeId = 0
	} else if node.lookupCount < 0 {
		log.Panicf("lookupCount underflow: %d: %v", node.lookupCount, c)
	}

	c.recursiveConsiderDropInode(node)
	node.treeLock.Unlock()
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

	if len(n.children) > 0 || n.lookupCount > 0 || !n.FsNode().Deletable() {
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
		node.treeLock.RLock()
		defer node.treeLock.RUnlock()
	}

	for i, component := range comps {
		if len(component) == 0 {
			continue
		}

		if node.mountPoint != nil {
			node.treeLock.RLock()
			defer node.treeLock.RUnlock()
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
		_, child, _ := c.internalLookup(parent, r, nil)
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
	parent.treeLock.Lock()
	defer parent.treeLock.Unlock()
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

	if parent.mounts == nil {
		parent.mounts = make(map[string]*fileSystemMount)
	}
	parent.mounts[name] = node.mountPoint
	node.mountPoint.parentInode = parent
	if c.Debug {
		log.Println("Mount: ", nodeFs, "on subdir", name,
			"parent", parent.nodeId)
	}
	c.verify()
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

	delete(parentNode.mounts, name)
	delete(parentNode.children, name)
	mount.fs.OnUnmount()

	c.EntryNotify(parentNode, name)

	return OK
}

func (c *FileSystemConnector) FileNotify(node *Inode, off int64, length int64) Status {
	n := node.nodeId
	if node == c.rootNode {
		n = raw.FUSE_ROOT_ID
	}
	if n == 0 {
		return OK
	}
	out := raw.NotifyInvalInodeOut{
		Length: length,
		Off:    off,
		Ino:    n,
	}
	return c.fsInit.InodeNotify(&out)
}

func (c *FileSystemConnector) EntryNotify(dir *Inode, name string) Status {
	n := dir.nodeId
	if dir == c.rootNode {
		n = raw.FUSE_ROOT_ID
	}
	if n == 0 {
		return OK
	}
	return c.fsInit.EntryNotify(n, name)
}
