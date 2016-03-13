package pathfs

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
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

	// protects pathInode.Parent pointers
	pathLock sync.RWMutex

	// This map lists all the parent links known for a given
	// nodeId.
	clientInodeMap clientInodeContainer

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
	if !fs.options.ClientInodes {
		return
	}
	fs.pathLock.Lock()
	fs.clientInodeMap = NewClientInodeContainer()
	fs.root.forgetClientInodes()
	fs.pathLock.Unlock()
}

// Rereads all inode numbers for all known files.
func (fs *PathNodeFs) RereadClientInodes() {
	if !fs.options.ClientInodes {
		return
	}
	fs.ForgetClientInodes()
	fs.root.updateClientInodes()
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
	return pNode.GetPath()
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
		clientInodeMap: NewClientInodeContainer(),
		options:        opts,
	}
	root.pathFs = pfs
	return pfs
}

// Root returns the root node for the path filesystem.
func (fs *PathNodeFs) Root() nodefs.Node {
	return fs.root
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes.
type pathInode struct {
	pathFs *PathNodeFs
	fs     FileSystem
	Name   string

	// This is nil at the root of the mount.
	Parent *pathInode

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

// Drop all known client inodes. Must have the treeLock.
func (n *pathInode) forgetClientInodes() {
	n.clientInode = 0
	for _, ch := range n.Inode().FsChildren() {
		ch.Node().(*pathInode).forgetClientInodes()
	}
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

// Reread all client nodes below this node.  Must run outside the treeLock.
func (n *pathInode) updateClientInodes() {
	n.GetAttr(&fuse.Attr{}, nil, nil)
	for _, ch := range n.Inode().FsChildren() {
		ch.Node().(*pathInode).updateClientInodes()
	}
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.
func (n *pathInode) GetPath() string {
	if n == n.pathFs.root {
		return ""
	}

	pathLen := 0

	// The simple solution is to collect names, and reverse join
	// them, them, but since this is a hot path, we take some
	// effort to avoid allocations.

	n.pathFs.pathLock.RLock()
	p := n
	for ; p.Parent != nil; p = p.Parent {
		pathLen += len(p.Name) + 1
	}
	pathLen--

	if p != p.pathFs.root {
		n.pathFs.pathLock.RUnlock()
		log.Printf("GetPath: hit deleted parent: n.Name=%s, p.Name=%s, p.IsDir=%v", n.Name, p.Name, p.Inode().IsDir())
		return ".deleted"
	}

	pathBytes := make([]byte, pathLen)
	end := len(pathBytes)
	for p = n; p.Parent != nil; p = p.Parent {
		l := len(p.Name)
		copy(pathBytes[end-l:], p.Name)
		end -= len(p.Name) + 1
		if end > 0 {
			pathBytes[end] = '/'
		}
	}
	n.pathFs.pathLock.RUnlock()

	path := string(pathBytes)
	if n.pathFs.debug {
		// TODO: print node ID.
		log.Printf("Inode = %q (%s)", path, n.fs.String())
	}

	return path
}

// addChild - set ourselves as the parent of the child and add it to
// clientInodeMap
func (n *pathInode) addChild(name string, child *pathInode) {
	child.Parent = n
	child.Name = name
	if !child.Inode().IsDir() {
		n.pathFs.clientInodeMap.add(child.clientInode, child, name, n)
	}
}

// rmChild - remove child "name"
func (n *pathInode) rmChild(name string) *pathInode {
	childInode := n.Inode().RmChild(name)
	fmt.Printf("n.Inode().RmChild(%s)\n", name)
	if childInode == nil {
		fmt.Printf("rmChild: Inode().RmChild(%s) returned nil\n", name)
		return nil
	}
	ch := childInode.Node().(*pathInode)

	// Is this the last hard link to this inode?
	// Directories cannot have hard links, to this is always true for directories.
	last := true
	if !ch.Inode().IsDir() {
		last = n.pathFs.clientInodeMap.rm(ch.clientInode, ch, name, n)
	}
	if last {
		// Mark the node as deleted as well. This is not strictly neccessary
		// but helps in debugging.
		ch.Name = ".deleted." + name
		ch.Parent = nil
		fmt.Printf("deleted entry %s\n", name)
	}

	return ch
}

func (n *pathInode) OnForget() {
	n.pathFs.pathLock.Lock()
	n.pathFs.clientInodeMap.drop(n.clientInode)
	n.pathFs.pathLock.Unlock()
}

////////////////////////////////////////////////////////////////
// FS operations

func (n *pathInode) StatFs() *fuse.StatfsOut {
	return n.fs.StatFs(n.GetPath())
}

func (n *pathInode) Readlink(c *fuse.Context) ([]byte, fuse.Status) {
	path := n.GetPath()

	val, err := n.fs.Readlink(path, c)
	return []byte(val), err
}

func (n *pathInode) Access(mode uint32, context *fuse.Context) (code fuse.Status) {
	p := n.GetPath()
	return n.fs.Access(p, mode, context)
}

func (n *pathInode) GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status) {
	return n.fs.GetXAttr(n.GetPath(), attribute, context)
}

func (n *pathInode) RemoveXAttr(attr string, context *fuse.Context) fuse.Status {
	p := n.GetPath()
	return n.fs.RemoveXAttr(p, attr, context)
}

func (n *pathInode) SetXAttr(attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	return n.fs.SetXAttr(n.GetPath(), attr, data, flags, context)
}

func (n *pathInode) ListXAttr(context *fuse.Context) (attrs []string, code fuse.Status) {
	return n.fs.ListXAttr(n.GetPath(), context)
}

func (n *pathInode) Flush(file nodefs.File, openFlags uint32, context *fuse.Context) (code fuse.Status) {
	return file.Flush()
}

func (n *pathInode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	return n.fs.OpenDir(n.GetPath(), context)
}

func (n *pathInode) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code := n.fs.Mknod(fullPath, mode, dev, context)
	var child *nodefs.Inode
	if code.Ok() {
		pNode := n.createChild(name, false)
		pNode.clientInode = n.getIno(fullPath, context)
		child = pNode.Inode()
		n.addChild(name, pNode)
	}
	return child, code
}

func (n *pathInode) Mkdir(name string, mode uint32, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code := n.fs.Mkdir(fullPath, mode, context)
	var child *nodefs.Inode
	if code.Ok() {
		pNode := n.createChild(name, true)
		child = pNode.Inode()
		n.addChild(name, pNode)
	}
	return child, code
}

func (n *pathInode) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	code = n.fs.Unlink(filepath.Join(n.GetPath(), name), context)
	if code.Ok() {
		n.rmChild(name)
	}
	return code
}

func (n *pathInode) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	code = n.fs.Rmdir(filepath.Join(n.GetPath(), name), context)
	if code.Ok() {
		n.rmChild(name)
	}
	return code
}

func (n *pathInode) Symlink(name string, content string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code := n.fs.Symlink(content, fullPath, context)
	var child *nodefs.Inode
	if code.Ok() {
		pNode := n.createChild(name, false)
		pNode.clientInode = n.getIno(fullPath, context)
		child = pNode.Inode()
		n.addChild(name, pNode)
	}
	return child, code
}

func (n *pathInode) Rename(oldName string, newParent nodefs.Node, newName string, context *fuse.Context) (code fuse.Status) {
	p := newParent.(*pathInode)
	oldPath := filepath.Join(n.GetPath(), oldName)
	newPath := filepath.Join(p.GetPath(), newName)
	code = n.fs.Rename(oldPath, newPath, context)
	if code.Ok() {
		ch := n.rmChild(oldName)
		p.rmChild(newName)
		p.Inode().AddChild(newName, ch.Inode())
		// Special case for UnionFS: A rename promotes a read-only file (no hard
		// link tracking) to a read-write file (hard links are supported, hence
		// inode must be set).
		if ch.clientInode == InoIgnore {
			ch.clientInode = n.getIno(newPath, context)
		}
		p.addChild(newName, ch)
	}
	return code
}

func (n *pathInode) Link(name string, existingFsnode nodefs.Node, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if !n.pathFs.options.ClientInodes {
		return nil, fuse.ENOSYS
	}

	newPath := filepath.Join(n.GetPath(), name)
	existing := existingFsnode.(*pathInode)
	oldPath := existing.GetPath()
	code := n.fs.Link(oldPath, newPath, context)

	// Special case for UnionFS: A link promotes a read-only file (no hard
	// link tracking) to a read-write file (hard links are supported, hence
	// inode must be set).
	if existing.clientInode == InoIgnore {
		existing.clientInode = n.getIno(oldPath, context)
		n.pathFs.clientInodeMap.add(existing.clientInode, existing, existing.Name, existing.Parent)
	}

	var a *fuse.Attr
	if code.Ok() {
		a, code = n.fs.GetAttr(newPath, context)
	}

	var child *nodefs.Inode
	if code.Ok() {
		if existing.clientInode != 0 && existing.clientInode == a.Ino {
			child = existing.Inode()
			n.Inode().AddChild(name, existing.Inode())
			n.addChild(name, existing)
		} else {
			pNode := n.createChild(name, false)
			child = pNode.Inode()
			pNode.clientInode = a.Ino
			n.addChild(name, pNode)
		}
	}
	return child, code
}

func (n *pathInode) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, *nodefs.Inode, fuse.Status) {
	var child *nodefs.Inode
	fullPath := filepath.Join(n.GetPath(), name)
	file, code := n.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		pNode := n.createChild(name, false)
		pNode.clientInode = n.getIno(fullPath, context)
		child = pNode.Inode()
		n.addChild(name, pNode)
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
			log.Panicf("getIno: GetAttr on %s returned ino=0, a=%v", fullPath, a)
		}
		return a.Ino
	} else {
		log.Printf("getIno: GetAttr on %s failed: %v", fullPath, code)
		return 0
	}
}


// createChild - create pathInode object and add it as a child to Inode()
func (n *pathInode) createChild(name string, isDir bool) *pathInode {
	i := new(pathInode)
	i.fs = n.fs
	i.pathFs = n.pathFs

	n.Inode().NewChild(name, isDir, i)
	return i
}

func (n *pathInode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	file, code = n.fs.Open(n.GetPath(), flags, context)
	if n.pathFs.debug {
		file = &nodefs.WithFlags{
			File:        file,
			Description: n.GetPath(),
		}
	}
	// Special case for UnionFS: A writeable open promotes a read-only file (no hard
	// link tracking) to a read-write file (hard links are supported, hence
	// inode must be set).
	if n.clientInode == InoIgnore && flags&fuse.O_ANYWRITE != 0 {
		n.clientInode = n.getIno(n.GetPath(), context)
		n.pathFs.clientInodeMap.add(n.clientInode, n, n.Name, n.Parent)
	}
	return
}

func (n *pathInode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (node *nodefs.Inode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	fi, code := n.fs.GetAttr(fullPath, context)
	if code.Ok() {
		node = n.findChild(fi, name, fullPath).Inode()
		*out = *fi
	}

	return node, code
}

// findChild - find or create a pathInode and add it as a child.
func (n *pathInode) findChild(fi *fuse.Attr, name string, fullPath string) (out *pathInode) {
	// Due to hard links, we may already know this inode
	if fi.Ino > 0 {
		n.pathFs.pathLock.RLock()
		v := n.pathFs.clientInodeMap.get(fi.Ino)
		if v != nil {
			out = v.node

			if fi.Nlink == 1 {
				// We know about other hard link(s), but the filesystem tells
				// us there is only one!?
				log.Println("Found linked inode, but Nlink == 1", fullPath)
			}
		}
		n.pathFs.pathLock.RUnlock()
	}

	if out == nil {
		out = n.createChild(name, fi.IsDir()) // This also calls Inode().AddChild
		out.clientInode = fi.Ino
	} else {
		n.Inode().AddChild(name, out.Inode())
	}
	n.addChild(name, out)

	return out
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
		fi, code = n.fs.GetAttr(n.GetPath(), context)
	}

	if fi != nil && !fi.IsDir() {
		n.pathFs.clientInodeMap.verify(fi.Ino, n)
	}

	if fi != nil && fi.Ino == InoIgnore {
		// We don't have a proper inode number. Set to zero to let
		// FileSystemConnector substitute the NodeId.
		fi.Ino = 0
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
		code = n.fs.Chmod(n.GetPath(), perms, context)
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
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		code = n.fs.Chown(n.GetPath(), uid, gid, context)
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
		code = n.fs.Truncate(n.GetPath(), size, context)
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
		code = n.fs.Utimens(n.GetPath(), atime, mtime, context)
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
