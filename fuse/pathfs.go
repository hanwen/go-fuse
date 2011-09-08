package fuse

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var _ = log.Println

type clientInodePath struct {
	parent *pathInode
	name   string
	node   *pathInode
}

type PathNodeFs struct {
	Debug bool
	fs   FileSystem
	root *pathInode
	connector *FileSystemConnector

	// Used for dealing with hardlinks.
	clientInodeMapMutex sync.Mutex
	clientInodeMap      map[uint64][]*clientInodePath
}

func (me *PathNodeFs) Mount(parent *Inode, name string, nodeFs NodeFileSystem, opts *FileSystemOptions) Status {
	return me.connector.Mount(parent, name, nodeFs, opts)
}

func (me *PathNodeFs) Unmount(node *Inode) Status {
	return me.connector.Unmount(node)
}

func (me *PathNodeFs) OnUnmount() {
}

func (me *PathNodeFs) OnMount(conn *FileSystemConnector) {
	me.connector = conn
	me.fs.OnMount(me)
}

func (me *PathNodeFs) StatFs() *StatfsOut {
	return me.fs.StatFs()
}

func (me *PathNodeFs) Node(name string) (*Inode) {
	n, rest := me.LastNode(name)
	if len(rest) > 0 {
		return nil
	}
	return n
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
	node := me.Node(path)
	if node == nil {
		return ENOENT
	}
	return me.connector.FileNotify(node, off, length)
}

func (me *PathNodeFs) EntryNotify(dir string, name string) Status {
	node := me.Node(dir)
	if node == nil {
		return ENOENT
	}
	return me.connector.EntryNotify(node, name)
}

func (me *PathNodeFs) Notify(path string) Status {
	node, rest := me.LastNode(path)
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

func NewPathNodeFs(fs FileSystem) *PathNodeFs {
	root := new(pathInode)
	root.fs = fs

	me := &PathNodeFs{
		fs:   fs,
		root: root,
		clientInodeMap: map[uint64][]*clientInodePath{},
	}
	root.ifs = me
	return me
}

func (me *PathNodeFs) Root() FsNode {
	return me.root
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes.
type pathInode struct {
	ifs  *PathNodeFs
	fs   FileSystem
	Name string

	// This is nil at the root of the mount.
	Parent *pathInode

	// This is to correctly resolve hardlinks of the underlying
	// real filesystem.
	clientInode uint64
	
	DefaultFsNode
}

func (me *pathInode) fillNewChildAttr(path string, child *pathInode, c *Context) (fi *os.FileInfo) {
	fi, _ = me.fs.GetAttr(path, c)
	if fi != nil && fi.Ino > 0 {
		child.clientInode = fi.Ino
	}
	return fi
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.  This will take the treeLock for the mount,
// so it can not be used in internal methods.
func (me *pathInode) GetPath() (path string) {
	defer me.inode.LockTree()()

	rev_components := make([]string, 0, 10)
	n := me
	for ; n.Parent != nil; n = n.Parent {
		rev_components = append(rev_components, n.Name)
	}
	if n != me.ifs.root {
		return ".deleted"
	}
	return ReverseJoin(rev_components, "/")
}

func (me *pathInode) AddChild(name string, child FsNode) {
	ch := child.(*pathInode)
	ch.Parent = me
	ch.Name = name

	if ch.clientInode > 0 {
		me.ifs.clientInodeMapMutex.Lock()
		defer me.ifs.clientInodeMapMutex.Unlock()
		m := me.ifs.clientInodeMap[ch.clientInode]
		e := &clientInodePath{
			me, name, child.(*pathInode),
		}
		m = append(m, e)
		me.ifs.clientInodeMap[ch.clientInode] = m
	}
}

func (me *pathInode) RmChild(name string, child FsNode) {
	ch := child.(*pathInode)

	if ch.clientInode > 0 {
		me.ifs.clientInodeMapMutex.Lock()
		defer me.ifs.clientInodeMapMutex.Unlock()
		m := me.ifs.clientInodeMap[ch.clientInode]

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
			return
		} else {
			me.ifs.clientInodeMap[ch.clientInode] = nil, false
		}
	} 
	
	ch.Name = ".deleted"
	ch.Parent = nil
}

func (me *pathInode) setClientInode(ino uint64) {
	if ino == me.clientInode {
		return
	}
	defer me.Inode().LockTree()()
	me.ifs.clientInodeMapMutex.Lock()
	defer me.ifs.clientInodeMapMutex.Unlock()
	if me.clientInode != 0 {
		me.ifs.clientInodeMap[me.clientInode] = nil, false
	}

	me.clientInode = ino
	if me.Parent != nil {
		e := &clientInodePath{
			me.Parent, me.Name, me,
		}
		me.ifs.clientInodeMap[ino] = append(me.ifs.clientInodeMap[ino], e)
	}
}

func (me *pathInode) OnForget() {
	if me.clientInode == 0 {
		return
	}
	me.ifs.clientInodeMapMutex.Lock()
	defer me.ifs.clientInodeMapMutex.Unlock()
	me.ifs.clientInodeMap[me.clientInode] = nil, false
}

////////////////////////////////////////////////////////////////
// FS operations


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
	code = file.Flush()
	if code.Ok() && openFlags&O_ANYWRITE != 0 {
		// We only signal releases to the FS if the
		// open could have changed things.
		path := me.GetPath()
		code = me.fs.Flush(path)
	}
	return code
}

func (me *pathInode) OpenDir(context *Context) (chan DirEntry, Status) {
	return me.fs.OpenDir(me.GetPath(), context)
}

func (me *pathInode) Mknod(name string, mode uint32, dev uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Mknod(fullPath, mode, dev, context)
	if code.Ok() {
		pNode := me.createChild(name)
		newNode = pNode
		fi = me.fillNewChildAttr(fullPath, pNode, context)
	}
	return
}

func (me *pathInode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Mkdir(fullPath, mode, context)
	if code.Ok() {
		pNode := me.createChild(name)
		newNode = pNode
		fi = me.fillNewChildAttr(fullPath, pNode, context)
	}
	return
}

func (me *pathInode) Unlink(name string, context *Context) (code Status) {
	return me.fs.Unlink(filepath.Join(me.GetPath(), name), context)
}

func (me *pathInode) Rmdir(name string, context *Context) (code Status) {
	return me.fs.Rmdir(filepath.Join(me.GetPath(), name), context)
}

func (me *pathInode) Symlink(name string, content string, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	code = me.fs.Symlink(content, fullPath, context)
	if code.Ok() {
		pNode := me.createChild(name)
		newNode = pNode
		fi = me.fillNewChildAttr(fullPath, pNode, context)
	}
	return
}

func (me *pathInode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	p := newParent.(*pathInode)
	oldPath := filepath.Join(me.GetPath(), oldName)
	newPath := filepath.Join(p.GetPath(), newName)

	return me.fs.Rename(oldPath, newPath, context)
}

func (me *pathInode) Link(name string, existing FsNode, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	newPath := filepath.Join(me.GetPath(), name)
	e := existing.(*pathInode)
	oldPath := e.GetPath()
	code = me.fs.Link(oldPath, newPath, context)
	if code.Ok() {
		fi, _ = me.fs.GetAttr(newPath, context)
		if fi != nil && e.clientInode != 0 && e.clientInode == fi.Ino {
			newNode = existing
		} else {
			newNode = me.createChild(name)
		}
	}
	return
}

func (me *pathInode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	file, code = me.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		pNode := me.createChild(name)
		newNode = pNode 
		fi = me.fillNewChildAttr(fullPath, pNode, context)
	}
	return
}

func (me *pathInode) createChild(name string) *pathInode {
	i := new(pathInode)
	i.Parent = me
	i.Name = name
	i.fs = me.fs
	i.ifs = me.ifs
	return i
}

func (me *pathInode) Open(flags uint32, context *Context) (file File, code Status) {
	return me.fs.Open(me.GetPath(), flags, context)
}

func (me *pathInode) Lookup(name string, context *Context) (fi *os.FileInfo, node FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	fi, code = me.fs.GetAttr(fullPath, context)
	if code.Ok() {
		node = me.findChild(fi.Ino, name)
	}

	return
}

func (me *pathInode) findChild(ino uint64, name string) (out *pathInode) {
	if ino > 0 {
		me.ifs.clientInodeMapMutex.Lock()
		defer me.ifs.clientInodeMapMutex.Unlock()
		v := me.ifs.clientInodeMap[ino]
		if len(v) > 0 {
			out = v[0].node
		}
	}

	if out == nil {
		out = me.createChild(name)
		out.clientInode = ino
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

	if file == nil || code == ENOSYS {
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
		if !code.Ok() {
			break
		}
	}

	if len(files) == 0 || code == ENOSYS {
		code = me.fs.Chmod(me.GetPath(), perms, context)
	}
	return code
}

func (me *pathInode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Chown(uid, gid)
		if !code.Ok() {
			break
		}
	}
	if len(files) == 0 || code == ENOSYS {
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
		if !code.Ok() {
			break
		}
	}
	if len(files) == 0 || code == ENOSYS {
		code = me.fs.Truncate(me.GetPath(), size, context)
	}
	return code
}

func (me *pathInode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	files := me.inode.Files(O_ANYWRITE)
	for _, f := range files {
		// TODO - pass context
		code = f.Utimens(atime, mtime)
		if !code.Ok() {
			break
		}
	}
	if len(files) == 0 || code == ENOSYS {
		code = me.fs.Utimens(me.GetPath(), atime, mtime, context)
	}
	return code
}
