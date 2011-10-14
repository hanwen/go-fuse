package fuse

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	Debug     bool
	fs        FileSystem
	root      *pathInode
	connector *FileSystemConnector

	// protects clientInodeMap and pathInode.Parent pointers
	pathLock sync.RWMutex

	// This map lists all the parent links known for a given
	// nodeId.
	clientInodeMap map[uint64][]*clientInodePath

	options *PathNodeFsOptions
}

type PathNodeFsOptions struct {
	// If ClientInodes is set, use Inode returned from GetAttr to
	// find hard-linked files.
	ClientInodes bool
}

func (me *PathNodeFs) Mount(path string, nodeFs NodeFileSystem, opts *FileSystemOptions) Status {
	dir, name := filepath.Split(path)
	if dir != "" {
		dir = filepath.Clean(dir)
	}
	parent := me.LookupNode(dir)
	if parent == nil {
		return ENOENT
	}
	return me.connector.Mount(parent, name, nodeFs, opts)
}

// Forgets all known information on client inodes.
func (me *PathNodeFs) ForgetClientInodes() {
	if !me.options.ClientInodes {
		return
	}
	me.pathLock.Lock()
	defer me.pathLock.Unlock()
	me.clientInodeMap = map[uint64][]*clientInodePath{}
	me.root.forgetClientInodes()
}

// Rereads all inode numbers for all known files.
func (me *PathNodeFs) RereadClientInodes() {
	if !me.options.ClientInodes {
		return
	}
	me.ForgetClientInodes()
	me.root.updateClientInodes()
}

func (me *PathNodeFs) UnmountNode(node *Inode) Status {
	return me.connector.Unmount(node)
}

func (me *PathNodeFs) Unmount(path string) Status {
	node := me.Node(path)
	if node == nil {
		return ENOENT
	}
	return me.connector.Unmount(node)
}

func (me *PathNodeFs) OnUnmount() {
}

func (me *PathNodeFs) String() string {
	return fmt.Sprintf("PathNodeFs(%v)", me.fs)
}

func (me *PathNodeFs) OnMount(conn *FileSystemConnector) {
	me.connector = conn
	me.fs.OnMount(me)
}

func (me *PathNodeFs) Node(name string) *Inode {
	n, rest := me.LastNode(name)
	if len(rest) > 0 {
		return nil
	}
	return n
}

// Like node, but use Lookup to discover inodes we may not have yet.
func (me *PathNodeFs) LookupNode(name string) *Inode {
	return me.connector.LookupNode(me.Root().Inode(), name)
}

func (me *PathNodeFs) Path(node *Inode) string {
	pNode := node.FsNode().(*pathInode)
	return pNode.GetPath()
}

func (me *PathNodeFs) LastNode(name string) (*Inode, []string) {
	if name == "" {
		return me.Root().Inode(), nil
	}

	name = filepath.Clean(name)
	comps := strings.Split(name, string(filepath.Separator))

	node := me.root.Inode()
	for i, c := range comps {
		next := node.GetChild(c)
		if next == nil {
			return node, comps[i:]
		}
		node = next
	}
	return node, nil
}

func (me *PathNodeFs) FileNotify(path string, off int64, length int64) Status {
	node, r := me.connector.Node(me.root.Inode(), path)
	if len(r) > 0 {
		return ENOENT
	}
	return me.connector.FileNotify(node, off, length)
}

func (me *PathNodeFs) EntryNotify(dir string, name string) Status {
	node, rest := me.connector.Node(me.root.Inode(), dir)
	if len(rest) > 0 {
		return ENOENT
	}
	return me.connector.EntryNotify(node, name)
}

func (me *PathNodeFs) Notify(path string) Status {
	node, rest := me.connector.Node(me.root.Inode(), path)
	if len(rest) > 0 {
		return me.connector.EntryNotify(node, rest[0])
	}
	return me.connector.FileNotify(node, 0, 0)
}

func (me *PathNodeFs) AllFiles(name string, mask uint32) []WithFlags {
	n := me.Node(name)
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

	me := &PathNodeFs{
		fs:             fs,
		root:           root,
		clientInodeMap: map[uint64][]*clientInodePath{},
		options:        opts,
	}
	root.pathFs = me
	return me
}

func (me *PathNodeFs) Root() FsNode {
	return me.root
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

	DefaultFsNode
}

// Drop all known client inodes. Must have the treeLock.
func (me *pathInode) forgetClientInodes() {
	me.clientInode = 0
	for _, ch := range me.Inode().FsChildren() {
		ch.FsNode().(*pathInode).forgetClientInodes()
	}
}

// Reread all client nodes below this node.  Must run outside the treeLock.
func (me *pathInode) updateClientInodes() {
	me.GetAttr(nil, nil)
	for _, ch := range me.Inode().FsChildren() {
		ch.FsNode().(*pathInode).updateClientInodes()
	}
}

func (me *pathInode) LockTree() func() {
	me.pathFs.pathLock.Lock()
	return func() { me.pathFs.pathLock.Unlock() }
}

func (me *pathInode) RLockTree() func() {
	me.pathFs.pathLock.RLock()
	return func() { me.pathFs.pathLock.RUnlock() }
}

func (me *pathInode) fillNewChildAttr(path string, child *pathInode, c *Context) (fi *os.FileInfo, code Status) {
	fi, _ = me.fs.GetAttr(path, c)
	if fi != nil && fi.Ino > 0 {
		child.clientInode = fi.Ino
	}

	if fi == nil {
		log.Println("fillNewChildAttr found nil FileInfo", path)
		return nil, ENOENT
	}
	return fi, OK
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.
func (me *pathInode) GetPath() (path string) {
	defer me.RLockTree()()

	rev_components := make([]string, 0, 10)
	n := me
	for ; n.Parent != nil; n = n.Parent {
		rev_components = append(rev_components, n.Name)
	}
	if n != me.pathFs.root {
		return ".deleted"
	}
	p := ReverseJoin(rev_components, "/")
	if me.pathFs.Debug {
		log.Printf("Inode %d = %q (%s)", me.Inode().nodeId, p, me.fs.String())
	}

	return p
}

func (me *pathInode) addChild(name string, child *pathInode) {
	me.Inode().AddChild(name, child.Inode())
	child.Parent = me
	child.Name = name

	if child.clientInode > 0 && me.pathFs.options.ClientInodes {
		defer me.LockTree()()
		m := me.pathFs.clientInodeMap[child.clientInode]
		e := &clientInodePath{
			me, name, child,
		}
		m = append(m, e)
		me.pathFs.clientInodeMap[child.clientInode] = m
	}
}

func (me *pathInode) rmChild(name string) *pathInode {
	childInode := me.Inode().RmChild(name)
	if childInode == nil {
		return nil
	}
	ch := childInode.FsNode().(*pathInode)

	if ch.clientInode > 0 && me.pathFs.options.ClientInodes {
		defer me.LockTree()()
		m := me.pathFs.clientInodeMap[ch.clientInode]

		idx := -1
		for i, v := range m {
			if v.parent == me && v.name == name {
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
			me.pathFs.clientInodeMap[ch.clientInode] = nil, false
		}
	}

	ch.Name = ".deleted"
	ch.Parent = nil

	return ch
}

// Handle a change in clientInode number for an other wise unchanged
// pathInode.
func (me *pathInode) setClientInode(ino uint64) {
	if ino == me.clientInode || !me.pathFs.options.ClientInodes {
		return
	}
	defer me.LockTree()()
	if me.clientInode != 0 {
		me.pathFs.clientInodeMap[me.clientInode] = nil, false
	}

	me.clientInode = ino
	if me.Parent != nil {
		e := &clientInodePath{
			me.Parent, me.Name, me,
		}
		me.pathFs.clientInodeMap[ino] = append(me.pathFs.clientInodeMap[ino], e)
	}
}

func (me *pathInode) OnForget() {
	if me.clientInode == 0 || !me.pathFs.options.ClientInodes {
		return
	}
	defer me.LockTree()()
	me.pathFs.clientInodeMap[me.clientInode] = nil, false
}

////////////////////////////////////////////////////////////////
// FS operations

func (me *pathInode) StatFs() *StatfsOut {
	return me.fs.StatFs(me.GetPath())
}

func (me *pathInode) Readlink(c *Context) ([]byte, Status) {
	path := me.GetPath()

	val, err := me.fs.Readlink(path, c)
	return []byte(val), err
}

func (me *pathInode) Access(mode uint32, context *Context) (code Status) {
	p := me.GetPath()
	return me.fs.Access(p, mode, context)
}

func (me *pathInode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return me.fs.GetXAttr(me.GetPath(), attribute, context)
}

func (me *pathInode) RemoveXAttr(attr string, context *Context) Status {
	p := me.GetPath()
	return me.fs.RemoveXAttr(p, attr, context)
}

func (me *pathInode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return me.fs.SetXAttr(me.GetPath(), attr, data, flags, context)
}

func (me *pathInode) ListXAttr(context *Context) (attrs []string, code Status) {
	return me.fs.ListXAttr(me.GetPath(), context)
}

func (me *pathInode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	return file.Flush()
}

func (me *pathInode) OpenDir(context *Context) (chan DirEntry, Status) {
	return me.fs.OpenDir(me.GetPath(), context)
}

func (me *pathInode) Mknod(name string, mode uint32, dev uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Mknod(fullPath, mode, dev, context)
	if code.Ok() {
		pNode := me.createChild(false)
		newNode = pNode
		fi, code = me.fillNewChildAttr(fullPath, pNode, context)
		me.addChild(name, pNode)
	}
	return
}

func (me *pathInode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Mkdir(fullPath, mode, context)
	if code.Ok() {
		pNode := me.createChild(true)
		newNode = pNode
		fi, code = me.fillNewChildAttr(fullPath, pNode, context)
		me.addChild(name, pNode)
	}
	return
}

func (me *pathInode) Unlink(name string, context *Context) (code Status) {
	code = me.fs.Unlink(filepath.Join(me.GetPath(), name), context)
	if code.Ok() {
		me.rmChild(name)
	}
	return code
}

func (me *pathInode) Rmdir(name string, context *Context) (code Status) {
	code = me.fs.Rmdir(filepath.Join(me.GetPath(), name), context)
	if code.Ok() {
		me.rmChild(name)
	}
	return code
}

func (me *pathInode) Symlink(name string, content string, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Symlink(content, fullPath, context)
	if code.Ok() {
		pNode := me.createChild(false)
		newNode = pNode
		fi, code = me.fillNewChildAttr(fullPath, pNode, context)
		me.addChild(name, pNode)
	}
	return
}

func (me *pathInode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	p := newParent.(*pathInode)
	oldPath := filepath.Join(me.GetPath(), oldName)
	newPath := filepath.Join(p.GetPath(), newName)
	code = me.fs.Rename(oldPath, newPath, context)
	if code.Ok() {
		ch := me.rmChild(oldName)
		p.rmChild(newName)
		p.addChild(newName, ch)
	}
	return code
}

func (me *pathInode) Link(name string, existingFsnode FsNode, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	if !me.pathFs.options.ClientInodes {
		return nil, nil, ENOSYS
	}

	newPath := filepath.Join(me.GetPath(), name)
	existing := existingFsnode.(*pathInode)
	oldPath := existing.GetPath()
	code = me.fs.Link(oldPath, newPath, context)
	if code.Ok() {
		fi, code = me.fs.GetAttr(newPath, context)
	}

	if code.Ok() {
		if existing.clientInode != 0 && existing.clientInode == fi.Ino {
			newNode = existing
			me.addChild(name, existing)
		} else {
			pNode := me.createChild(false)
			newNode = pNode
			pNode.clientInode = fi.Ino
			me.addChild(name, pNode)
		}
	}
	return
}

func (me *pathInode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	file, code = me.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		pNode := me.createChild(false)
		newNode = pNode
		fi, code = me.fillNewChildAttr(fullPath, pNode, context)
		me.addChild(name, pNode)
	}
	return
}

func (me *pathInode) createChild(isDir bool) *pathInode {
	i := new(pathInode)
	i.fs = me.fs
	i.pathFs = me.pathFs

	me.Inode().New(isDir, i)
	return i
}

func (me *pathInode) Open(flags uint32, context *Context) (file File, code Status) {
	file, code = me.fs.Open(me.GetPath(), flags, context)
	if me.pathFs.Debug {
		file = &WithFlags{
			File:        file,
			Description: me.GetPath(),
		}
	}
	return
}

func (me *pathInode) Lookup(name string, context *Context) (fi *os.FileInfo, node FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	fi, code = me.fs.GetAttr(fullPath, context)
	if code.Ok() {
		node = me.findChild(fi, name, fullPath)
	}

	return
}

func (me *pathInode) findChild(fi *os.FileInfo, name string, fullPath string) (out *pathInode) {
	if fi.Ino > 0 {
		unlock := me.RLockTree()
		v := me.pathFs.clientInodeMap[fi.Ino]
		if len(v) > 0 {
			out = v[0].node

			if fi.Nlink == 1 {
				log.Println("Found linked inode, but Nlink == 1", fullPath)
			}
		}
		unlock()
	}

	if out == nil {
		out = me.createChild(fi.IsDirectory())
		out.clientInode = fi.Ino
		me.addChild(name, out)
	}

	return out
}

func (me *pathInode) GetAttr(file File, context *Context) (fi *os.FileInfo, code Status) {
	if file == nil {
		// called on a deleted files.
		file = me.inode.AnyFile()
	}

	if file != nil {
		fi, code = file.GetAttr()
	}

	if file == nil || code == ENOSYS || code == EBADF {
		fi, code = me.fs.GetAttr(me.GetPath(), context)
	}

	if fi != nil {
		me.setClientInode(fi.Ino)
	}

	if fi != nil && !fi.IsDirectory() && fi.Nlink == 0 {
		fi.Nlink = 1
	}
	return fi, code
}

func (me *pathInode) Chmod(file File, perms uint32, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Chmod(perms)
		if code.Ok() {
			return
		}
	}

	if len(files) == 0 || code == ENOSYS || code == EBADF {
		code = me.fs.Chmod(me.GetPath(), perms, context)
	}
	return code
}

func (me *pathInode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Chown(uid, gid)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == ENOSYS || code == EBADF {
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		code = me.fs.Chown(me.GetPath(), uid, gid, context)
	}
	return code
}

func (me *pathInode) Truncate(file File, size uint64, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Truncate(size)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == ENOSYS || code == EBADF {
		code = me.fs.Truncate(me.GetPath(), size, context)
	}
	return code
}

func (me *pathInode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Utimens(atime, mtime)
		if code.Ok() {
			return code
		}
	}
	if len(files) == 0 || code == ENOSYS || code == EBADF {
		code = me.fs.Utimens(me.GetPath(), atime, mtime, context)
	}
	return code
}
