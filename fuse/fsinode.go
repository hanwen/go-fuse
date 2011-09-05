package fuse
import (
	"log"
	"os"
	"path/filepath"
)

var _ = log.Println

// This is a combination of dentry (entry in the file/directory and
// the inode). This structure is used to implement glue for FSes where
// there is a one-to-one mapping of paths and inodes, ie. FSes that
// disallow hardlinks.
type fsInode struct {
	*inode
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

	if me.inode.mount == nil {
		// Node from unmounted file system.
		return ".deleted"
	}

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

////////////////////////////////////////////////////////////////

func (me *fsInode) Readlink(c *Context) ([]byte, Status) {
	path := me.GetPath()
	
	val, err := me.inode.mount.fs.Readlink(path, c)
	return []byte(val), err
}

func (me *fsInode) Access(mode uint32, context *Context) (code Status) {
	p := me.GetPath()
	return me.inode.mount.fs.Access(p, mode, context)
}

func (me *fsInode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return me.inode.mount.fs.GetXAttr(me.GetPath(), attribute, context)
}

func (me *fsInode) RemoveXAttr(attr string, context *Context) Status {
	p := me.GetPath()
	return me.inode.mount.fs.RemoveXAttr(p, attr, context)
}

func (me *fsInode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return me.inode.mount.fs.SetXAttr(me.GetPath(), attr, data, flags, context)
}

func (me *fsInode) ListXAttr(context *Context) (attrs []string, code Status) {
	return me.inode.mount.fs.ListXAttr(me.GetPath(), context)
}

func (me *fsInode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	code = file.Flush()
	if code.Ok() && openFlags&O_ANYWRITE != 0 {
		// We only signal releases to the FS if the
		// open could have changed things.
		path := me.GetPath()
		code = me.inode.mount.fs.Flush(path)
	}
	return code
}

func (me *fsInode) OpenDir(context *Context) (chan DirEntry, Status) {
	return me.inode.mount.fs.OpenDir(me.GetPath(), context)
}

func (me *fsInode) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	p := me.GetPath()
	return me.inode.mount.fs.Mknod(filepath.Join(p, name), mode, dev, context)
}
	
func (me *fsInode) Mkdir(name string, mode uint32, context *Context) (code Status) {
	return me.inode.mount.fs.Mkdir(filepath.Join(me.GetPath(), name), mode, context)
}

func (me *fsInode) Unlink(name string, context *Context) (code Status) {
	return me.inode.mount.fs.Unlink(filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Rmdir(name string, context *Context) (code Status) {
	return me.inode.mount.fs.Rmdir(filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Symlink(name string, content string, context *Context) (code Status) {
	return me.inode.mount.fs.Symlink(content, filepath.Join(me.GetPath(), name), context)
}


func (me *fsInode) Rename(oldName string, newParent *fsInode, newName string, context *Context) (code Status) {
	oldPath := filepath.Join(me.GetPath(), oldName)
	newPath := filepath.Join(newParent.GetPath(), newName)
	
	return me.inode.mount.fs.Rename(oldPath, newPath, context)
}

func (me *fsInode) Link(name string, existing *fsInode, context *Context) (code Status) {
	return me.inode.mount.fs.Link(existing.GetPath(), filepath.Join(me.GetPath(), name), context)
}

func (me *fsInode) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	return me.inode.mount.fs.Create(fullPath, flags, mode, context)
}

func (me *fsInode) Open(flags uint32, context *Context) (file File, code Status) {
	return me.inode.mount.fs.Open(me.GetPath(), flags, context)
}

// TODO: should return fsInode.
func (me *fsInode) Lookup(name string) (fi *os.FileInfo, code Status) {
	fullPath := filepath.Join(me.GetPath(), name)
	return me.inode.mount.fs.GetAttr(fullPath, nil)
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
		fi, code = me.inode.mount.fs.GetAttr(me.GetPath(), context)
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
		code = me.inode.mount.fs.Chmod(me.GetPath(), perms, context)
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
		code = me.inode.mount.fs.Chown(me.GetPath(), uid, gid, context)
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
		code = me.inode.mount.fs.Truncate(me.GetPath(), size, context)
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
		code = me.inode.mount.fs.Utimens(me.GetPath(), atime, mtime, context)
	}
	return code
}
