package pathfs

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

// PathNodeFs is the file system that can translate an inode back to a
// path.  The path name is then used to call into an object that has
// the FileSystem interface.
//
// Lookups (ie. FileSystem.GetAttr) may return a inode number in its
// return value. The inode number ("clientInode") is used to indicate
// linked files.
type PathNodeFs struct {
	debug     bool
	fs        FileSystem
	root      *pathInode
	connector *nodefs.FileSystemConnector

	// Any operation that modifies the filesystem tree must Lock(), any operation
	// that reads the filesystem tree (parents maps or clientInodeMap) must RLock()
	// pathLock. It serves three purposes:
	// (1) protect the maps from concurrent access
	// (2) hide intermediate inconsistent states from other threads (example: Rename)
	// (3) make sure the tree does not change between getPath and the actual fs
	//     operation (example: StatFs)
	pathLock sync.RWMutex

	// Files may have hard links, and we want that file to be represented by only
	// one pathInode. This map allows to get the pathInode given the inode
	// number, if this file is already known.
	clientInodeMap map[uint64]*pathInode

	options *PathNodeFsOptions
}

// SetDebug toggles debug information: it will log path names for
// each operation processed.
func (fs *PathNodeFs) SetDebug(dbg bool) {
	fs.debug = dbg
}

// Mount mounts a another node filesystem with the given root on the
// path. The last component of the path should not exist yet.
func (fs *PathNodeFs) Mount(path string, root nodefs.Node, opts *nodefs.Options) fuse.Status {
	dir, name := filepath.Split(path)
	if dir != "" {
		dir = filepath.Clean(dir)
	}
	parent := fs.LookupNode(dir)
	if parent == nil {
		return fuse.ENOENT
	}
	return fs.connector.Mount(parent, name, root, opts)
}

// ForgetClientInodes forgets all known information on client inodes.
func (fs *PathNodeFs) ForgetClientInodes() {
	fs.pathLock.Lock()
	fs.clientInodeMap = map[uint64]*pathInode{}
	fs.pathLock.Unlock()
}

// UnmountNode unmounts the node filesystem with the given root.
func (fs *PathNodeFs) UnmountNode(node *nodefs.Inode) fuse.Status {
	return fs.connector.Unmount(node)
}

// UnmountNode unmounts the node filesystem with the given root.
func (fs *PathNodeFs) Unmount(path string) fuse.Status {
	node := fs.Node(path)
	if node == nil {
		return fuse.ENOENT
	}
	return fs.connector.Unmount(node)
}

// String returns a name for this file system
func (fs *PathNodeFs) String() string {
	name := fs.fs.String()
	if name == "defaultFileSystem" {
		name = fmt.Sprintf("%T", fs.fs)
		name = strings.TrimLeft(name, "*")
	}
	return name
}

// Connector returns the FileSystemConnector (the bridge to the raw
// protocol) for this PathNodeFs.
func (fs *PathNodeFs) Connector() *nodefs.FileSystemConnector {
	return fs.connector
}

// Node looks up the Inode that corresponds to the given path name, or
// returns nil if not found.
func (fs *PathNodeFs) Node(name string) *nodefs.Inode {
	n, rest := fs.LastNode(name)
	if len(rest) > 0 {
		return nil
	}
	return n
}

// Like Node, but use Lookup to discover inodes we may not have yet.
func (fs *PathNodeFs) LookupNode(name string) *nodefs.Inode {
	return fs.connector.LookupNode(fs.Root().Inode(), name)
}

// Path constructs a path for the given Inode. If the file system
// implements hard links through client-inode numbers, the path may
// not be unique.
func (fs *PathNodeFs) Path(node *nodefs.Inode) string {
	pNode := node.Node().(*pathInode)
	return pNode.getPath()
}

// LastNode finds the deepest inode known corresponding to a path. The
// unknown part of the filename is also returned.
func (fs *PathNodeFs) LastNode(name string) (*nodefs.Inode, []string) {
	return fs.connector.Node(fs.Root().Inode(), name)
}

// FileNotify notifies that file contents were changed within the
// given range.  Use negative offset for metadata-only invalidation,
// and zero-length for invalidating all content.
func (fs *PathNodeFs) FileNotify(path string, off int64, length int64) fuse.Status {
	node, r := fs.connector.Node(fs.root.Inode(), path)
	if len(r) > 0 {
		return fuse.ENOENT
	}
	return fs.connector.FileNotify(node, off, length)
}

// EntryNotify makes the kernel forget the entry data from the given
// name from a directory.  After this call, the kernel will issue a
// new lookup request for the given name when necessary.
func (fs *PathNodeFs) EntryNotify(dir string, name string) fuse.Status {
	node, rest := fs.connector.Node(fs.root.Inode(), dir)
	if len(rest) > 0 {
		return fuse.ENOENT
	}
	return fs.connector.EntryNotify(node, name)
}

// Notify ensures that the path name is invalidates: if the inode is
// known, it issues an file content Notify, if not, an entry notify
// for the path is issued. The latter will clear out non-existence
// cache entries.
func (fs *PathNodeFs) Notify(path string) fuse.Status {
	node, rest := fs.connector.Node(fs.root.Inode(), path)
	if len(rest) > 0 {
		return fs.connector.EntryNotify(node, rest[0])
	}
	return fs.connector.FileNotify(node, 0, 0)
}

// AllFiles returns all open files for the inode corresponding with
// the given mask.
func (fs *PathNodeFs) AllFiles(name string, mask uint32) []nodefs.WithFlags {
	n := fs.Node(name)
	if n == nil {
		return nil
	}
	return n.Files(mask)
}

// NewPathNodeFs returns a file system that translates from inodes to
// path names.
func NewPathNodeFs(fs FileSystem, opts *PathNodeFsOptions) *PathNodeFs {
	root := new(pathInode)
	root.fs = fs

	if opts == nil {
		opts = &PathNodeFsOptions{}
	}

	pfs := &PathNodeFs{
		fs:             fs,
		root:           root,
		clientInodeMap: map[uint64]*pathInode{},
		options:        opts,
	}
	root.pathFs = pfs
	return pfs
}

// Root returns the root node for the path filesystem.
func (fs *PathNodeFs) Root() nodefs.Node {
	return fs.root
}

type nameAndParent struct {
	name   string
	parent *pathInode
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes.
type pathInode struct {
	pathFs *PathNodeFs
	fs     FileSystem

	// Due to hard links, a file can have many parents and one ore more names
	// under each parent. The map is indexed by (name, parent) tuples, the
	// value is unused.
	// The filesystem root directory has no parents and this map is empty.
	parents map[nameAndParent]bool

	// This is to correctly resolve hardlinks of the underlying
	// real filesystem.
	clientInode uint64
	inode       *nodefs.Inode
}

func (n *pathInode) OnMount(conn *nodefs.FileSystemConnector) {
	n.pathFs.connector = conn
	n.pathFs.fs.OnMount(n.pathFs)
}

func (n *pathInode) OnUnmount() {
}

func (fs *pathInode) Deletable() bool {
	return true
}

func (n *pathInode) Inode() *nodefs.Inode {
	return n.inode
}

func (n *pathInode) SetInode(node *nodefs.Inode) {
	n.inode = node
}

// getParent - returns a random parent of this pathInode and the name under this
// parent.
// The caller must hold pathLock.
func (n *pathInode) getParent() (*pathInode, string) {
	for k, _ := range n.parents {
		return k.parent, k.name
	}
	return nil, ""
}

// GetPath returns a path relative to the mount pointing to this pathInode.
// Due to hard links, a file may have many paths. This function returns a
// random one.
// Must be called with pathLock held.
func (n *pathInode) getPath() string {
	if n == n.pathFs.root {
		return ""
	}
	if len(n.parents) == 0 {
		dummyPath := fmt.Sprintf(".ERROR_NO_PARENTS.%d", n.clientInode)
		log.Printf("getPath: non-root node without parents, returning dummyPath=%q", dummyPath)
		return dummyPath
	}

	// The simple solution is to collect names, and reverse join
	// them. This is a hot path and may need optimization to avoid
	// too many allocations.

	// Walk up to the root and collect names in pathSegments
	var pathSegments []string
	var nextName string
	nextParent := n
	for len(nextParent.parents) > 0 {
		nextParent, nextName = nextParent.getParent()
		pathSegments = append(pathSegments, nextName)

		if len(pathSegments) > syscall.PathMax {
			dummyPath := fmt.Sprintf(".ERROR_DIR_LOOP.%d", n.clientInode)
			log.Printf("getPath: over %d path segments, directory loop? returning dummyPath=%q",
				syscall.PathMax, dummyPath)
			log.Printf("First 10 path segments: %v", pathSegments[0:9])
			return dummyPath
		}
	}
	// Join segments to full path
	fullPath := pathSegments[0]
	if len(pathSegments) > 1 {
		for _, segment := range pathSegments[1:] {
			fullPath = segment + "/" + fullPath
		}
	}
	// Sanity check: At the end of the walk we should be at the root node
	if nextParent != n.pathFs.root {
		dummyPath := fmt.Sprintf(".ERROR_WRONG_ROOT.%d", n.clientInode)
		log.Printf("getPath: hit wrong root node, fullPath=%s, returning dummyPath=%q", fullPath, dummyPath)
		return dummyPath
	}
	if n.pathFs.debug {
		// TODO: print node ID.
		log.Printf("Inode = %s (%s)", fullPath, n.fs.String())
	}

	return fullPath
}

// addChild - make ourselves a parent of the child and add it to n.Inode()
// The caller must hold pathLock.
func (n *pathInode) addChild(name string, child *pathInode) {
	child.parents[nameAndParent{name, n}] = true
	// Inform nodefs.Inode of the added child
	n.Inode().AddChild(name, child.Inode())
}

// rmChild - remove child "name"
// The caller must hold pathLock.
func (n *pathInode) rmChild(name string) *pathInode {
	childInode := n.Inode().RmChild(name)
	if childInode == nil {
		return nil
	}
	ch := childInode.Node().(*pathInode)
	delete(ch.parents, nameAndParent{name, n})
	return ch
}

func (n *pathInode) OnForget() {
	n.pathFs.pathLock.Lock()
	// Remove from inode map
	delete(n.pathFs.clientInodeMap, n.clientInode)
	// Remove ourself from all parents
	for entry, _ := range n.parents {
		entry.parent.Inode().RmChild(entry.name)
	}
	n.parents = nil
	n.pathFs.pathLock.Unlock()
}

////////////////////////////////////////////////////////////////
// FS operations

func (n *pathInode) StatFs() *fuse.StatfsOut {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.StatFs(n.getPath())
}

func (n *pathInode) Readlink(c *fuse.Context) ([]byte, fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	path := n.getPath()

	val, err := n.fs.Readlink(path, c)
	return []byte(val), err
}

func (n *pathInode) Access(mode uint32, context *fuse.Context) (code fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.Access(n.getPath(), mode, context)
}

func (n *pathInode) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.GetXAttr(n.getPath(), attribute, context)
}

func (n *pathInode) RemoveXAttr(attr string, context *fuse.Context) fuse.Status {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	p := n.getPath()
	return n.fs.RemoveXAttr(p, attr, context)
}

func (n *pathInode) SetXAttr(attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.SetXAttr(n.getPath(), attr, data, flags, context)
}

func (n *pathInode) ListXAttr(context *fuse.Context) (attrs []string, code fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.ListXAttr(n.getPath(), context)
}

func (n *pathInode) Flush(file nodefs.File, openFlags uint32, context *fuse.Context) (code fuse.Status) {
	return file.Flush()
}

func (n *pathInode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	return n.fs.OpenDir(n.getPath(), context)
}

func (n *pathInode) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	fullPath := filepath.Join(n.getPath(), name)
	var child *nodefs.Inode
	code := n.fs.Mknod(fullPath, mode, dev, context)
	if code.Ok() {
		ino := n.getIno(fullPath, context)
		pNode := n.createChild(name, false, ino)
		child = pNode.Inode()
	}
	return child, code
}

func (n *pathInode) Mkdir(name string, mode uint32, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	fullPath := filepath.Join(n.getPath(), name)
	code := n.fs.Mkdir(fullPath, mode, context)
	var child *nodefs.Inode
	if code.Ok() {
		pNode := n.createChild(name, true, 0)
		child = pNode.Inode()
	}
	return child, code
}

func (n *pathInode) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	code = n.fs.Unlink(filepath.Join(n.getPath(), name), context)
	if code.Ok() {
		n.rmChild(name)
	}
	return code
}

func (n *pathInode) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	code = n.fs.Rmdir(filepath.Join(n.getPath(), name), context)
	if code.Ok() {
		n.rmChild(name)
	}
	return code
}

func (n *pathInode) Symlink(name string, content string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	fullPath := filepath.Join(n.getPath(), name)
	var child *nodefs.Inode
	code := n.fs.Symlink(content, fullPath, context)
	if code.Ok() {
		ino := n.getIno(fullPath, context)
		pNode := n.createChild(name, false, ino)
		child = pNode.Inode()
	}
	return child, code
}

func (n *pathInode) Rename(oldName string, newParent nodefs.Node, newName string, context *fuse.Context) (code fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	p := newParent.(*pathInode)
	oldPath := filepath.Join(n.getPath(), oldName)
	newPath := filepath.Join(p.getPath(), newName)
	code = n.fs.Rename(oldPath, newPath, context)
	if code.Ok() {
		ch := n.rmChild(oldName)
		p.rmChild(newName)
		p.addChild(newName, ch)
	}
	return code
}

func (n *pathInode) Link(name string, existingFsnode nodefs.Node, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if !n.pathFs.options.ClientInodes {
		return nil, fuse.ENOSYS
	}
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	newPath := filepath.Join(n.getPath(), name)
	existing := existingFsnode.(*pathInode)
	oldPath := existing.getPath()
	code := n.fs.Link(oldPath, newPath, context)
	if code.Ok() {
		n.addChild(name, existing)
	}
	return existingFsnode.Inode(), code
}

func (n *pathInode) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, *nodefs.Inode, fuse.Status) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	var child *nodefs.Inode
	fullPath := filepath.Join(n.getPath(), name)
	file, code := n.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		ino := n.getIno(fullPath, context)
		pNode := n.createChild(name, false, ino)
		child = pNode.Inode()
	}
	return file, child, code
}

// getIno - retrieve the inode number of "fullPath"
// If ClientInodes are not enabled, this is a no-op that returns 0.
func (n *pathInode) getIno(fullPath string, context *fuse.Context) uint64 {
	if !n.pathFs.options.ClientInodes {
		return 0
	}
	var a *fuse.Attr
	a, code := n.fs.GetAttr(fullPath, context)
	if code.Ok() {
		if a.Ino == 0 {
			log.Printf("getIno: GetAttr on %q returned ino=0", fullPath)
		}
		return a.Ino
	} else {
		log.Printf("getIno: GetAttr on %q failed: %v", fullPath, code)
		return 0
	}
}

// createChild - create a new pathInode object, add it to clientInodeMap and our
// nodefs.Inode and adopt it.
// The caller must hold pathLock.
func (n *pathInode) createChild(name string, isDir bool, ino uint64) *pathInode {
	child := &pathInode{
		fs:          n.fs,
		pathFs:      n.pathFs,
		clientInode: ino,
		parents:     map[nameAndParent]bool{},
	}
	if ino != 0 && !isDir {
		n.pathFs.clientInodeMap[ino] = child
	}
	// Make ourself a parent of the child
	child.parents[nameAndParent{name, n}] = true
	// Inform nodefs.Inode of the new child
	n.Inode().NewChild(name, isDir, child)
	return child
}

func (n *pathInode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	file, code = n.fs.Open(n.getPath(), flags, context)
	if n.pathFs.debug {
		file = &nodefs.WithFlags{
			File:        file,
			Description: n.getPath(),
		}
	}
	return
}

func (n *pathInode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (node *nodefs.Inode, code fuse.Status) {
	n.pathFs.pathLock.RLock()
	defer n.pathFs.pathLock.RUnlock()

	fullPath := filepath.Join(n.getPath(), name)
	fi, code := n.fs.GetAttr(fullPath, context)
	if code.Ok() {
		*out = *fi
		var child *pathInode
		if !fi.IsDir() {
			child = n.pathFs.clientInodeMap[fi.Ino]
		}
		if child != nil {
			n.addChild(name, child)
			if fi.Nlink == 1 {
				// We just found another hard link, but the filesystem tells us
				// there is only one!?
				log.Printf("Found linked inode, but Nlink == 1: %q", fullPath)
			}
		} else {
			child = n.createChild(name, fi.IsDir(), fi.Ino)
		}
		node = child.Inode()
	}
	return node, code
}

func (n *pathInode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	var fi *fuse.Attr
	if file == nil {
		// Linux currently (tested on v4.4) does not pass a file descriptor for
		// fstat. To be able to stat a deleted file we have to find ourselves
		// an open fd.
		file = n.Inode().AnyFile()
	}

	if file != nil {
		code = file.GetAttr(out)
	}

	if file == nil || code == fuse.ENOSYS || code == fuse.EBADF {
		n.pathFs.pathLock.RLock()
		fi, code = n.fs.GetAttr(n.getPath(), context)
		n.pathFs.pathLock.RUnlock()
	}

	// Lazy inode update. Unionfs depends on this when files are promoted
	// from ro to rw.
	if fi != nil && n.clientInode == 0 && fi.Ino != 0 {
		n.clientInode = fi.Ino
		n.pathFs.pathLock.Lock()
		n.pathFs.clientInodeMap[fi.Ino] = n
		n.pathFs.pathLock.Unlock()
	}

	if fi != nil && !fi.IsDir() && fi.Nlink == 0 {
		fi.Nlink = 1
	}

	if fi != nil {
		*out = *fi
	}
	return code
}

func (n *pathInode) Chmod(file nodefs.File, perms uint32, context *fuse.Context) (code fuse.Status) {
	// Note that Linux currently (Linux 4.4) DOES NOT pass a file descriptor
	// to FUSE for fchmod. We still check because that may change in the future.
	if file != nil {
		code = file.Chmod(perms)
		if code != fuse.ENOSYS {
			return code
		}
	}

	files := n.Inode().Files(fuse.O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Chmod(perms)
		if code.Ok() {
			return
		}
	}

	if len(files) == 0 || code == fuse.ENOSYS || code == fuse.EBADF {
		n.pathFs.pathLock.RLock()
		code = n.fs.Chmod(n.getPath(), perms, context)
		n.pathFs.pathLock.RUnlock()
	}
	return code
}

func (n *pathInode) Chown(file nodefs.File, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	// Note that Linux currently (Linux 4.4) DOES NOT pass a file descriptor
	// to FUSE for fchown. We still check because it may change in the future.
	if file != nil {
		code = file.Chown(uid, gid)
		if code != fuse.ENOSYS {
			return code
		}
	}

	files := n.Inode().Files(fuse.O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Chown(uid, gid)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == fuse.ENOSYS || code == fuse.EBADF {
		n.pathFs.pathLock.RLock()
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		code = n.fs.Chown(n.getPath(), uid, gid, context)
		n.pathFs.pathLock.RUnlock()
	}
	return code
}

func (n *pathInode) Truncate(file nodefs.File, size uint64, context *fuse.Context) (code fuse.Status) {
	// A file descriptor was passed in AND the filesystem implements the
	// operation on the file handle. This the common case for ftruncate.
	if file != nil {
		code = file.Truncate(size)
		if code != fuse.ENOSYS {
			return code
		}
	}

	files := n.Inode().Files(fuse.O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Truncate(size)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == fuse.ENOSYS || code == fuse.EBADF {
		n.pathFs.pathLock.RLock()
		code = n.fs.Truncate(n.getPath(), size, context)
		n.pathFs.pathLock.RUnlock()
	}
	return code
}

func (n *pathInode) Utimens(file nodefs.File, atime *time.Time, mtime *time.Time, context *fuse.Context) (code fuse.Status) {
	// Note that Linux currently (Linux 4.4) DOES NOT pass a file descriptor
	// to FUSE for futimens. We still check because it may change in the future.
	if file != nil {
		code = file.Utimens(atime, mtime)
		if code != fuse.ENOSYS {
			return code
		}
	}

	files := n.Inode().Files(fuse.O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Utimens(atime, mtime)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == fuse.ENOSYS || code == fuse.EBADF {
		n.pathFs.pathLock.RLock()
		code = n.fs.Utimens(n.getPath(), atime, mtime, context)
		n.pathFs.pathLock.RUnlock()
	}
	return code
}

func (n *pathInode) Fallocate(file nodefs.File, off uint64, size uint64, mode uint32, context *fuse.Context) (code fuse.Status) {
	if file != nil {
		code = file.Allocate(off, size, mode)
		if code.Ok() {
			return code
		}
	}

	files := n.Inode().Files(fuse.O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Allocate(off, size, mode)
		if code.Ok() {
			return code
		}
	}

	return code
}

func (n *pathInode) Read(file nodefs.File, dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
	if file != nil {
		return file.Read(dest, off)
	}
	return nil, fuse.ENOSYS
}

func (n *pathInode) Write(file nodefs.File, data []byte, off int64, context *fuse.Context) (written uint32, code fuse.Status) {
	if file != nil {
		return file.Write(data, off)
	}
	return 0, fuse.ENOSYS
}
