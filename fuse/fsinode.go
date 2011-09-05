package fuse

import (
	"log"
	"os"
	"path/filepath"
)

var _ = log.Println

type inodeFs struct {
	fs   FileSystem
	root *fsInode
}

func (me *inodeFs) Unmount() {
}

func (me *inodeFs) Mount(conn *FileSystemConnector) {
	me.fs.Mount(conn)
}

func (me *inodeFs) StatFs() *StatfsOut {
	return me.fs.StatFs()
}

func newInodeFs(fs FileSystem) *inodeFs {
	root := new(fsInode)
	root.fs = fs
	
	me := &inodeFs{
	fs: fs,
	root: root,
	}
	root.ifs = me
	return me
}

func (me *inodeFs) RootNode() *fsInode {
	return me.root
}

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes, ie. FSes that
// disallow hardlinks.
type fsInode struct {
	*inode
	ifs    *inodeFs
	fs     FileSystem
	Name   string

	// This is nil at the root of the mount.
	Parent *fsInode
}

// GetPath returns the path relative to the mount governing this
// inode.  It returns nil for mount if the file was deleted or the
// filesystem unmounted.  This will take the treeLock for the mount,
// so it can not be used in internal methods.
func (me *fsInode) GetPath() (path string) {
	me.inode.treeLock.RLock()
	defer me.inode.treeLock.RUnlock()

	rev_components := make([]string, 0, 10)
	n := me
	for ; n.Parent != nil; n = n.Parent {
		rev_components = append(rev_components, n.Name)
	}
	if n.mountPoint == nil {
		return ".deleted"
	}
	return ReverseJoin(rev_components, "/")
}

func (me *fsInode) addChild(name string, ch *fsInode) {
	if ch.inode.mountPoint == nil {
		ch.Parent = me
	}
	ch.Name = name
}

func (me *fsInode) rmChild(name string, ch *fsInode) {
	ch.Name = ".deleted"
	ch.Parent = nil
}

func (me *fsInode) SetInode(node *inode) {
	if me.inode != nil {
		panic("already have inode")
	}
	me.inode = node
}

////////////////////////////////////////////////////////////////

func (me *fsInode) Readlink(c *Context) ([]byte, Status) {
	path := me.GetPath()
	
	val, err := me.fs.Readlink(path, c)
	return []byte(val), err
}

func (me *fsInode) Access(mode uint32, context *Context) (code Status) {
	p := me.GetPath()
	return me.fs.Access(p, mode, context)
}

func (me *fsInode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return me.fs.GetXAttr(me.GetPath(), attribute, context)
}

func (me *fsInode) RemoveXAttr(attr string, context *Context) Status {
	p := me.GetPath()
	return me.fs.RemoveXAttr(p, attr, context)
}

func (me *fsInode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return me.fs.SetXAttr(me.GetPath(), attr, data, flags, context)
}

func (me *fsInode) ListXAttr(context *Context) (attrs []string, code Status) {
	return me.fs.ListXAttr(me.GetPath(), context)
}

func (me *fsInode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	code = file.Flush()
	if code.Ok() && openFlags&O_ANYWRITE != 0 {
		// We only signal releases to the FS if the
		// open could have changed things.
		path := me.GetPath()
		code = me.fs.Flush(path)
	}
	return code
}

func (me *fsInode) OpenDir(context *Context) (chan DirEntry, Status) {
	return me.fs.OpenDir(me.GetPath(), context)
}

func (me *fsInode) Mknod(name string, mode uint32, dev uint32, context *Context) (fi *os.FileInfo, newNode *fsInode, code Status) {
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
	
func (me *fsInode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode *fsInode, code Status) {
	code = me.fs.Mkdir(filepath.Join(me.GetPath(), name), mode, context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFDIR | mode,
		}
	}
	return
}

func (me *fsInode) Unlink(name string, context *Context) (code Status) {
	return me.fs.Unlink(filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Rmdir(name string, context *Context) (code Status) {
	return me.fs.Rmdir(filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Symlink(name string, content string, context *Context) (fi *os.FileInfo, newNode *fsInode, code Status) {
	code = me.fs.Symlink(content, filepath.Join(me.GetPath(), name), context)
	if code.Ok() {
		newNode = me.createChild(name)
		fi = &os.FileInfo{
			Mode: S_IFLNK | 0666, // TODO
		}
	}
	return
}


func (me *fsInode) Rename(oldName string, newParent *fsInode, newName string, context *Context) (code Status) {
	oldPath := filepath.Join(me.GetPath(), oldName)
	newPath := filepath.Join(newParent.GetPath(), newName)
	
	return me.fs.Rename(oldPath, newPath, context)
}

func (me *fsInode) Link(name string, existing *fsInode, context *Context) (code Status) {
	return me.fs.Link(existing.GetPath(), filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode *fsInode, code Status) {
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

func (me *fsInode) createChild(name string) *fsInode {
	i := new(fsInode)
	i.Parent = me
	i.Name = name
	i.fs = me.fs
	i.ifs = me.ifs
	return i
}

func (me *fsInode) Open(flags uint32, context *Context) (file File, code Status) {
	return me.fs.Open(me.GetPath(), flags, context)
}

// TOOD - need context.
func (me *fsInode) Lookup(name string) (fi *os.FileInfo, node *fsInode, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	fi, code = me.fs.GetAttr(fullPath, nil)
	if code.Ok() {
		node = me.createChild(name)
	}

	return
}

func (me *fsInode) GetAttr(file File, context *Context) (fi *os.FileInfo, code Status) {
	if file == nil {
		// called on a deleted files.
		file = me.inode.getAnyFile()
	}
	
	if file != nil {
		fi, code = file.GetAttr()
	}

	if file == nil || code == ENOSYS {
		fi, code = me.fs.GetAttr(me.GetPath(), context)
	}
	
	if fi != nil && !fi.IsDirectory() {
		fi.Nlink = 1
	}
	return fi, code
}	

func (me *fsInode) Chmod(file File, perms uint32, context *Context) (code Status) {
	files := me.inode.getWritableFiles()
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

	
func (me *fsInode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	files := me.inode.getWritableFiles()
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

func (me *fsInode) Truncate(file File, size uint64, context *Context) (code Status) {
	files := me.inode.getWritableFiles()
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

func (me *fsInode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	files := me.inode.getWritableFiles()
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
