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

// A parent pointer: node should be reachable as parent.children[name]
type clientInodePath struct {
	parent *pathInode
	name   string
	node   *pathInode
}

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

	// protects pathInode.parents
	pathLock sync.RWMutex

	// This map lists all the parent links known for a given
	// nodeId.
	clientInodeMap map[uint64][]*clientInodePath

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
	fs.clientInodeMap = map[uint64][]*clientInodePath{}
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
	root := &pathInode{
		parents: map[parentData]struct{}{},
	}
	root.fs = fs

	if opts == nil {
		opts = &PathNodeFsOptions{}
	}

	pfs := &PathNodeFs{
		fs:             fs,
		root:           root,
		clientInodeMap: map[uint64][]*clientInodePath{},
		options:        opts,
	}
	root.pathFs = pfs
	return pfs
}

// Root returns the root node for the path filesystem.
func (fs *PathNodeFs) Root() nodefs.Node {
	return fs.root
}

type parentData struct {
	parent *pathInode
	name   string
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes.
type pathInode struct {
	pathFs *PathNodeFs
	fs     FileSystem

	parents map[parentData]struct{}

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

// TODO - return *parentData?
func (n *pathInode) parent() parentData {
	for k := range n.parents {
		return k
	}
	return parentData{}
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

	pathLen := 1

	// The simple solution is to collect names, and reverse join
	// them, them, but since this is a hot path, we take some
	// effort to avoid allocations.

	n.pathFs.pathLock.RLock()
	walkUp := n

	// TODO - guess depth? use *parentData?
	parents := make([]parentData, 0, 10)
	for {
		p := walkUp.parent()
		if p.parent == nil {
			break
		}
		parents = append(parents, p)
		pathLen += len(p.name) + 1
		walkUp = p.parent
	}
	pathLen--

	pathBytes := make([]byte, 0, pathLen)
	for i := len(parents) - 1; i >= 0; i-- {
		pathBytes = append(pathBytes, parents[i].name...)
		if i > 0 {
			pathBytes = append(pathBytes, '/')
		}
	}
	n.pathFs.pathLock.RUnlock()

	path := string(pathBytes)
	if n.pathFs.debug {
		log.Printf("Inode = %q (%s)", path, n.fs.String())
	}

	if walkUp != n.pathFs.root {
		// This might happen if the node has been removed from
		// the tree using unlink, but we are forced to run
		// some file system operation, because the file is
		// still opened.

		// TODO - add a deterministic disambiguating suffix.
		return ".deleted"
	}

	return path
}

func (n *pathInode) OnAdd(parent *nodefs.Inode, name string) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	pathParent := parent.Node().(*pathInode)
	n.parents[parentData{pathParent, name}] = struct{}{}

	if n.clientInode > 0 && n.pathFs.options.ClientInodes {
		m := n.pathFs.clientInodeMap[n.clientInode]
		e := &clientInodePath{
			pathParent, name, n,
		}
		m = append(m, e)
		n.pathFs.clientInodeMap[n.clientInode] = m
	}
}

func (n *pathInode) rmChild(name string) *pathInode {
	childInode := n.Inode().RmChild(name)
	if childInode == nil {
		return nil
	}
	return childInode.Node().(*pathInode)
}

func (n *pathInode) OnRemove(parent *nodefs.Inode, name string) {
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()

	// TODO - paranoia: what if the cast fails? Can this happen?
	parentPI := parent.Node().(*pathInode)

	delete(n.parents, parentData{parentPI, name})

	if n.clientInode > 0 && n.pathFs.options.ClientInodes {
		m := n.pathFs.clientInodeMap[n.clientInode]

		idx := -1
		// Find the right entry: both "parent" and "name" must match
		for i, v := range m {
			if v.parent == parentPI && v.name == name {
				idx = i
				break
			}
		}
		if idx >= 0 {
			// Delete the "idx" entry from the middle of the slice by moving the
			// last element over it and truncating the slice
			m[idx] = m[len(m)-1]
			m = m[:len(m)-1]
			n.pathFs.clientInodeMap[n.clientInode] = m
		}
		if len(m) == 0 {
			delete(n.pathFs.clientInodeMap, n.clientInode)
		}
	}
}

// Handle a change in clientInode number for an other wise unchanged
// pathInode.
func (n *pathInode) setClientInode(ino uint64) {
	if ino == n.clientInode || !n.pathFs.options.ClientInodes {
		return
	}
	n.pathFs.pathLock.Lock()
	defer n.pathFs.pathLock.Unlock()
	if n.clientInode != 0 {
		delete(n.pathFs.clientInodeMap, n.clientInode)
	}

	n.clientInode = ino
	if p := n.parent(); p.parent != nil {
		e := &clientInodePath{
			p.parent, p.name, n,
		}
		n.pathFs.clientInodeMap[ino] = append(n.pathFs.clientInodeMap[ino], e)
	}
}

func (n *pathInode) OnForget() {
	if n.clientInode == 0 || !n.pathFs.options.ClientInodes {
		return
	}
	n.pathFs.pathLock.Lock()
	delete(n.pathFs.clientInodeMap, n.clientInode)
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
		child = pNode.Inode()
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
	}
	return child, code
}

func (n *pathInode) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	code = n.fs.Unlink(filepath.Join(n.GetPath(), name), context)
	if code.Ok() {
		n.Inode().RmChild(name)
	}
	return code
}

func (n *pathInode) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	code = n.fs.Rmdir(filepath.Join(n.GetPath(), name), context)
	if code.Ok() {
		n.Inode().RmChild(name)
	}
	return code
}

func (n *pathInode) Symlink(name string, content string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code := n.fs.Symlink(content, fullPath, context)
	var child *nodefs.Inode
	if code.Ok() {
		pNode := n.createChild(name, false)
		child = pNode.Inode()
	}
	return child, code
}

func (n *pathInode) Rename(oldName string, newParent nodefs.Node, newName string, context *fuse.Context) (code fuse.Status) {
	p := newParent.(*pathInode)
	oldPath := filepath.Join(n.GetPath(), oldName)
	newPath := filepath.Join(p.GetPath(), newName)
	code = n.fs.Rename(oldPath, newPath, context)
	if code.Ok() {
		ch := n.Inode().RmChild(oldName)
		p.Inode().AddChild(newName, ch)
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

	var a *fuse.Attr
	if code.Ok() {
		a, code = n.fs.GetAttr(newPath, context)
	}

	var child *nodefs.Inode
	if code.Ok() {
		if existing.clientInode != 0 && existing.clientInode == a.Ino {
			child = existing.Inode()
			n.Inode().AddChild(name, existing.Inode())
		} else {
			pNode := n.createChild(name, false)
			child = pNode.Inode()
			pNode.clientInode = a.Ino
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
		child = pNode.Inode()
	}
	return file, child, code
}

func (n *pathInode) createChild(name string, isDir bool) *pathInode {
	i := &pathInode{
		parents: map[parentData]struct{}{},
	}

	i.fs = n.fs
	i.pathFs = n.pathFs

	n.Inode().NewChild(name, isDir, i)
	return i
}

func (n *pathInode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	p := n.GetPath()
	file, code = n.fs.Open(p, flags, context)
	if n.pathFs.debug {
		file = &nodefs.WithFlags{
			File:        file,
			Description: n.GetPath(),
		}
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

func (n *pathInode) findChild(fi *fuse.Attr, name string, fullPath string) (out *pathInode) {
	if fi.Ino > 0 {
		n.pathFs.pathLock.RLock()
		v := n.pathFs.clientInodeMap[fi.Ino]
		if len(v) > 0 {
			out = v[0].node

			if fi.Nlink == 1 {
				log.Println("Found linked inode, but Nlink == 1", fullPath)
			}
		}
		n.pathFs.pathLock.RUnlock()
	}

	if out == nil {
		out = n.createChild(name, fi.IsDir())
		out.clientInode = fi.Ino
	} else {
		// should add 'out' as a child to n ?
	}
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

	if fi != nil {
		n.setClientInode(fi.Ino)
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
