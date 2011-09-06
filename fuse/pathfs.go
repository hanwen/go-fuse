package fuse

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

var _ = log.Println

type PathNodeFs struct {
	fs   FileSystem
	root *pathInode
	connector *FileSystemConnector
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

func (me *PathNodeFs) Node(name string) *Inode {
	name = filepath.Clean(name)
	comps := strings.Split(name, string(filepath.Separator))
	node := me.root.Inode()
	for _, c := range comps {
		node = node.GetChild(c)
		if node == nil {
			break
		}
	}
	return node
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
	}
	root.ifs = me
	return me
}

func (me *PathNodeFs) Root() FsNode {
	return me.root
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes, ie. FSes that
// disallow hardlinks.
type pathInode struct {
	ifs  *PathNodeFs
	fs   FileSystem
	Name string

	// This is nil at the root of the mount.
	Parent *pathInode

	DefaultFsNode
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
	if ch.inode.mountPoint == nil {
		ch.Parent = me
	} else {
		log.Printf("name %q", name)
		panic("should have no mounts")
	}
	ch.Name = name
}

func (me *pathInode) RmChild(name string, child FsNode) {
	ch := child.(*pathInode)
	ch.Name = ".deleted"
	ch.Parent = nil
}

////////////////////////////////////////////////////////////////

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
	p := me.GetPath()
	code = me.fs.Mknod(filepath.Join(p, name), mode, dev, context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFIFO | mode, // TODO
		}
	}
	return
}

func (me *pathInode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	code = me.fs.Mkdir(filepath.Join(me.GetPath(), name), mode, context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFDIR | mode,
		}
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
	code = me.fs.Symlink(content, filepath.Join(me.GetPath(), name), context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFLNK | 0666, // TODO
		}
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
		oldFi, _ := me.fs.GetAttr(oldPath, context)
		fi, _ = me.fs.GetAttr(newPath, context)
		if oldFi != nil && fi != nil && oldFi.Ino != 0 && oldFi.Ino == fi.Ino {
			return fi, existing, OK
		}
		newNode = me.createChild(name)
	}
	return
}

func (me *pathInode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	file, code = me.fs.Create(fullPath, flags, mode, context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFREG | mode,
			// TODO - ctime, mtime, atime?
		}
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

// TOOD - need context.
func (me *pathInode) Lookup(name string) (fi *os.FileInfo, node FsNode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	fi, code = me.fs.GetAttr(fullPath, nil)
	if code.Ok() {
		node = me.createChild(name)
	}

	return
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
		log.Println("truncating file", f)
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
