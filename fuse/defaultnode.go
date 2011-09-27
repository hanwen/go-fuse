package fuse

import (
	"log"
	"os"
)

var _ = log.Println

type DefaultNodeFileSystem struct {

}

func (me *DefaultNodeFileSystem) OnUnmount() {
}

func (me *DefaultNodeFileSystem) OnMount(conn *FileSystemConnector) {

}

func (me *DefaultNodeFileSystem) Root() FsNode {
	return new(DefaultFsNode)
}

func (me *DefaultNodeFileSystem) String() string {
	return "DefaultNodeFileSystem"
}

////////////////////////////////////////////////////////////////
// FsNode default

type DefaultFsNode struct {
	inode *Inode
}

func (me *DefaultFsNode) StatFs() *StatfsOut {
	return nil
}

func (me *DefaultFsNode) SetInode(node *Inode) {
	if me.inode != nil {
		panic("already have Inode")
	}
	me.inode = node
}

func (me *DefaultFsNode) Deletable() bool {
	return true
}

func (me *DefaultFsNode) Inode() *Inode {
	return me.inode
}

func (me *DefaultFsNode) OnForget() {
}

func (me *DefaultFsNode) Lookup(name string, context *Context) (fi *os.FileInfo, node FsNode, code Status) {
	return nil, nil, ENOENT
}

func (me *DefaultFsNode) Access(mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Readlink(c *Context) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultFsNode) Mknod(name string, mode uint32, dev uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}
func (me *DefaultFsNode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}
func (me *DefaultFsNode) Unlink(name string, context *Context) (code Status) {
	return ENOSYS
}
func (me *DefaultFsNode) Rmdir(name string, context *Context) (code Status) {
	return ENOSYS
}
func (me *DefaultFsNode) Symlink(name string, content string, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (me *DefaultFsNode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Link(name string, existing FsNode, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (me *DefaultFsNode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode FsNode, code Status) {
	return nil, nil, nil, ENOSYS
}

func (me *DefaultFsNode) Open(flags uint32, context *Context) (file File, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFsNode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) OpenDir(context *Context) (chan DirEntry, Status) {
	ch := me.Inode().Children()
	s := make(chan DirEntry, len(ch))
	for name, child := range ch {
		fi, code := child.FsNode().GetAttr(nil, context)
		if code.Ok() {
			s <- DirEntry{Name: name, Mode: fi.Mode}
		}
	}
	close(s)
	return s, OK
}

func (me *DefaultFsNode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFsNode) RemoveXAttr(attr string, context *Context) Status {
	return ENOSYS
}

func (me *DefaultFsNode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return ENOSYS
}

func (me *DefaultFsNode) ListXAttr(context *Context) (attrs []string, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFsNode) GetAttr(file File, context *Context) (fi *os.FileInfo, code Status) {
	if me.Inode().IsDir() {
		return &os.FileInfo{Mode: S_IFDIR | 0755}, OK
	}
	return &os.FileInfo{Mode: S_IFREG | 0644}, OK
}

func (me *DefaultFsNode) Chmod(file File, perms uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Truncate(file File, size uint64, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	return ENOSYS
}
