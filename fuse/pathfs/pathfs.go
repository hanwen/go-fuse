package pathfs

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

var _ = log.Println

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
// linked files. The clientInode is never exported back to the kernel;
// it is only used to maintain a list of all names of an inode.
type PathNodeFs struct {
	debug     bool
	fs        FileSystem
	root      *pathInode
	connector *fuse.FileSystemConnector

	// protects clientInodeMap and pathInode.Parent pointers
	pathLock sync.RWMutex

	// This map lists all the parent links known for a given
	// nodeId.
	clientInodeMap map[uint64][]*clientInodePath

	options *PathNodeFsOptions
}

func (fs *PathNodeFs) SetDebug(dbg bool) {
	fs.debug = dbg
}

func (fs *PathNodeFs) Mount(path string, nodeFs fuse.NodeFileSystem, opts *fuse.FileSystemOptions) fuse.Status {
	dir, name := filepath.Split(path)
	if dir != "" {
		dir = filepath.Clean(dir)
	}
	parent := fs.LookupNode(dir)
	if parent == nil {
		return fuse.ENOENT
	}
	return fs.connector.Mount(parent, name, nodeFs, opts)
}

// Forgets all known information on client inodes.
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

func (fs *PathNodeFs) UnmountNode(node *fuse.Inode) fuse.Status {
	return fs.connector.Unmount(node)
}

func (fs *PathNodeFs) Unmount(path string) fuse.Status {
	node := fs.Node(path)
	if node == nil {
		return fuse.ENOENT
	}
	return fs.connector.Unmount(node)
}

func (fs *PathNodeFs) OnUnmount() {
}

func (fs *PathNodeFs) String() string {
	name := fs.fs.String()
	if name == "DefaultFileSystem" {
		name = fmt.Sprintf("%T", fs.fs)
		name = strings.TrimLeft(name, "*")
	}
	return name
}

func (fs *PathNodeFs) OnMount(conn *fuse.FileSystemConnector) {
	fs.connector = conn
	fs.fs.OnMount(fs)
}

func (fs *PathNodeFs) Node(name string) *fuse.Inode {
	n, rest := fs.LastNode(name)
	if len(rest) > 0 {
		return nil
	}
	return n
}

// Like node, but use Lookup to discover inodes we may not have yet.
func (fs *PathNodeFs) LookupNode(name string) *fuse.Inode {
	return fs.connector.LookupNode(fs.Root().Inode(), name)
}

func (fs *PathNodeFs) Path(node *fuse.Inode) string {
	pNode := node.FsNode().(*pathInode)
	return pNode.GetPath()
}

func (fs *PathNodeFs) LastNode(name string) (*fuse.Inode, []string) {
	return fs.connector.Node(fs.Root().Inode(), name)
}

func (fs *PathNodeFs) FileNotify(path string, off int64, length int64) fuse.Status {
	node, r := fs.connector.Node(fs.root.Inode(), path)
	if len(r) > 0 {
		return fuse.ENOENT
	}
	return fs.connector.FileNotify(node, off, length)
}

func (fs *PathNodeFs) EntryNotify(dir string, name string) fuse.Status {
	node, rest := fs.connector.Node(fs.root.Inode(), dir)
	if len(rest) > 0 {
		return fuse.ENOENT
	}
	return fs.connector.EntryNotify(node, name)
}

func (fs *PathNodeFs) Notify(path string) fuse.Status {
	node, rest := fs.connector.Node(fs.root.Inode(), path)
	if len(rest) > 0 {
		return fs.connector.EntryNotify(node, rest[0])
	}
	return fs.connector.FileNotify(node, 0, 0)
}

func (fs *PathNodeFs) AllFiles(name string, mask uint32) []fuse.WithFlags {
	n := fs.Node(name)
	if n == nil {
		return nil
	}
	return n.Files(mask)
}

func NewPathNodeFs(fs FileSystem, opts *PathNodeFsOptions) *PathNodeFs {
	root := new(pathInode)
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

func (fs *PathNodeFs) Root() fuse.FsNode {
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
	inode      *fuse.Inode
}

// Drop all known client inodes. Must have the treeLock.
func (n *pathInode) forgetClientInodes() {
	n.clientInode = 0
	for _, ch := range n.Inode().FsChildren() {
		ch.FsNode().(*pathInode).forgetClientInodes()
	}
}

func (fs *pathInode) Deletable() bool {
	return true
}

func (n *pathInode) Inode() *fuse.Inode {
	return n.inode
}

func (n *pathInode) SetInode(node *fuse.Inode) {
	n.inode = node
}

// Reread all client nodes below this node.  Must run outside the treeLock.
func (n *pathInode) updateClientInodes() {
	n.GetAttr(&fuse.Attr{}, nil, nil)
	for _, ch := range n.Inode().FsChildren() {
		ch.FsNode().(*pathInode).updateClientInodes()
	}
}

func (n *pathInode) LockTree() func() {
	n.pathFs.pathLock.Lock()
	return func() { n.pathFs.pathLock.Unlock() }
}

func (n *pathInode) RLockTree() func() {
	n.pathFs.pathLock.RLock()
	return func() { n.pathFs.pathLock.RUnlock() }
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

func (n *pathInode) addChild(name string, child *pathInode) {
	n.Inode().AddChild(name, child.Inode())
	child.Parent = n
	child.Name = name

	if child.clientInode > 0 && n.pathFs.options.ClientInodes {
		defer n.LockTree()()
		m := n.pathFs.clientInodeMap[child.clientInode]
		e := &clientInodePath{
			n, name, child,
		}
		m = append(m, e)
		n.pathFs.clientInodeMap[child.clientInode] = m
	}
}

func (n *pathInode) rmChild(name string) *pathInode {
	childInode := n.Inode().RmChild(name)
	if childInode == nil {
		return nil
	}
	ch := childInode.FsNode().(*pathInode)

	if ch.clientInode > 0 && n.pathFs.options.ClientInodes {
		defer n.LockTree()()
		m := n.pathFs.clientInodeMap[ch.clientInode]

		idx := -1
		for i, v := range m {
			if v.parent == n && v.name == name {
				idx = i
				break
			}
		}
		if idx >= 0 {
			m[idx] = m[len(m)-1]
			m = m[:len(m)-1]
		}
		if len(m) > 0 {
			ch.Parent = m[0].parent
			ch.Name = m[0].name
			return ch
		} else {
			delete(n.pathFs.clientInodeMap, ch.clientInode)
		}
	}

	ch.Name = ".deleted"
	ch.Parent = nil

	return ch
}

// Handle a change in clientInode number for an other wise unchanged
// pathInode.
func (n *pathInode) setClientInode(ino uint64) {
	if ino == n.clientInode || !n.pathFs.options.ClientInodes {
		return
	}
	defer n.LockTree()()
	if n.clientInode != 0 {
		delete(n.pathFs.clientInodeMap, n.clientInode)
	}

	n.clientInode = ino
	if n.Parent != nil {
		e := &clientInodePath{
			n.Parent, n.Name, n,
		}
		n.pathFs.clientInodeMap[ino] = append(n.pathFs.clientInodeMap[ino], e)
	}
}

func (n *pathInode) OnForget() {
	if n.clientInode == 0 || !n.pathFs.options.ClientInodes {
		return
	}
	defer n.LockTree()()
	delete(n.pathFs.clientInodeMap, n.clientInode)
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

func (n *pathInode) Flush(file fuse.File, openFlags uint32, context *fuse.Context) (code fuse.Status) {
	return file.Flush()
}

func (n *pathInode) OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	return n.fs.OpenDir(n.GetPath(), context)
}

func (n *pathInode) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (newNode fuse.FsNode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code = n.fs.Mknod(fullPath, mode, dev, context)
	if code.Ok() {
		pNode := n.createChild(false)
		newNode = pNode
		n.addChild(name, pNode)
	}
	return
}

func (n *pathInode) Mkdir(name string, mode uint32, context *fuse.Context) (newNode fuse.FsNode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code = n.fs.Mkdir(fullPath, mode, context)
	if code.Ok() {
		pNode := n.createChild(true)
		newNode = pNode
		n.addChild(name, pNode)
	}
	return
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

func (n *pathInode) Symlink(name string, content string, context *fuse.Context) (newNode fuse.FsNode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	code = n.fs.Symlink(content, fullPath, context)
	if code.Ok() {
		pNode := n.createChild(false)
		newNode = pNode
		n.addChild(name, pNode)
	}
	return
}

func (n *pathInode) Rename(oldName string, newParent fuse.FsNode, newName string, context *fuse.Context) (code fuse.Status) {
	p := newParent.(*pathInode)
	oldPath := filepath.Join(n.GetPath(), oldName)
	newPath := filepath.Join(p.GetPath(), newName)
	code = n.fs.Rename(oldPath, newPath, context)
	if code.Ok() {
		ch := n.rmChild(oldName)
		p.rmChild(newName)
		p.addChild(newName, ch)
	}
	return code
}

func (n *pathInode) Link(name string, existingFsnode fuse.FsNode, context *fuse.Context) (newNode fuse.FsNode, code fuse.Status) {
	if !n.pathFs.options.ClientInodes {
		return nil, fuse.ENOSYS
	}

	newPath := filepath.Join(n.GetPath(), name)
	existing := existingFsnode.(*pathInode)
	oldPath := existing.GetPath()
	code = n.fs.Link(oldPath, newPath, context)

	var a *fuse.Attr
	if code.Ok() {
		a, code = n.fs.GetAttr(newPath, context)
	}

	if code.Ok() {
		if existing.clientInode != 0 && existing.clientInode == a.Ino {
			newNode = existing
			n.addChild(name, existing)
		} else {
			pNode := n.createChild(false)
			newNode = pNode
			pNode.clientInode = a.Ino
			n.addChild(name, pNode)
		}
	}
	return
}

func (n *pathInode) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, newNode fuse.FsNode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	file, code = n.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		pNode := n.createChild(false)
		newNode = pNode
		n.addChild(name, pNode)
	}
	return
}

func (n *pathInode) createChild(isDir bool) *pathInode {
	i := new(pathInode)
	i.fs = n.fs
	i.pathFs = n.pathFs

	n.Inode().New(isDir, i)
	return i
}

func (n *pathInode) Open(flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	file, code = n.fs.Open(n.GetPath(), flags, context)
	if n.pathFs.debug {
		file = &fuse.WithFlags{
			File:        file,
			Description: n.GetPath(),
		}
	}
	return
}

func (n *pathInode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (node fuse.FsNode, code fuse.Status) {
	fullPath := filepath.Join(n.GetPath(), name)
	fi, code := n.fs.GetAttr(fullPath, context)
	if code.Ok() {
		node = n.findChild(fi, name, fullPath)
		*out = *fi
	}

	return node, code
}

func (n *pathInode) findChild(fi *fuse.Attr, name string, fullPath string) (out *pathInode) {
	if fi.Ino > 0 {
		unlock := n.RLockTree()
		v := n.pathFs.clientInodeMap[fi.Ino]
		if len(v) > 0 {
			out = v[0].node

			if fi.Nlink == 1 {
				log.Println("Found linked inode, but Nlink == 1", fullPath)
			}
		}
		unlock()
	}

	if out == nil {
		out = n.createChild(fi.IsDir())
		out.clientInode = fi.Ino
		n.addChild(name, out)
	}

	return out
}

func (n *pathInode) GetAttr(out *fuse.Attr, file fuse.File, context *fuse.Context) (code fuse.Status) {
	var fi *fuse.Attr
	if file == nil {
		// called on a deleted files.
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

func (n *pathInode) Chmod(file fuse.File, perms uint32, context *fuse.Context) (code fuse.Status) {
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

func (n *pathInode) Chown(file fuse.File, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
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

func (n *pathInode) Truncate(file fuse.File, size uint64, context *fuse.Context) (code fuse.Status) {
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

func (n *pathInode) Utimens(file fuse.File, atime *time.Time, mtime *time.Time, context *fuse.Context) (code fuse.Status) {
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

func (n *pathInode) Fallocate(file fuse.File, off uint64, size uint64, mode uint32, context *fuse.Context) (code fuse.Status) {
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
