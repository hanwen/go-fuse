package fuse

import (
	"log"
	"time"
)

var _ = log.Println

type DefaultNodeFileSystem struct {
}

func (fs *DefaultNodeFileSystem) OnUnmount() {
}

func (fs *DefaultNodeFileSystem) OnMount(conn *FileSystemConnector) {

}

func (fs *DefaultNodeFileSystem) Root() FsNode {
	return new(DefaultFsNode)
}

func (fs *DefaultNodeFileSystem) String() string {
	return "DefaultNodeFileSystem"
}

////////////////////////////////////////////////////////////////
// FsNode default

type DefaultFsNode struct {
	inode *Inode
}

var _ = FsNode((*DefaultFsNode)(nil))

func (n *DefaultFsNode) StatFs() *StatfsOut {
	return nil
}

func (n *DefaultFsNode) SetInode(node *Inode) {
	n.inode = node
}

func (n *DefaultFsNode) Deletable() bool {
	return true
}

func (n *DefaultFsNode) Inode() *Inode {
	return n.inode
}

func (n *DefaultFsNode) OnForget() {
}

func (n *DefaultFsNode) Lookup(out *Attr, name string, context *Context) (node FsNode, code Status) {
	return nil, ENOENT
}

func (n *DefaultFsNode) Access(mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Readlink(c *Context) ([]byte, Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) Mknod(name string, mode uint32, dev uint32, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}
func (n *DefaultFsNode) Mkdir(name string, mode uint32, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}
func (n *DefaultFsNode) Unlink(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *DefaultFsNode) Rmdir(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *DefaultFsNode) Symlink(name string, content string, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Link(name string, existing FsNode, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) Create(name string, flags uint32, mode uint32, context *Context) (file File, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (n *DefaultFsNode) Open(flags uint32, context *Context) (file File, code Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) OpenDir(context *Context) ([]DirEntry, Status) {
	ch := n.Inode().Children()
	s := make([]DirEntry, 0, len(ch))
	for name, child := range ch {
		var a Attr
		code := child.FsNode().GetAttr(&a, nil, context)
		if code.Ok() {
			s = append(s, DirEntry{Name: name, Mode: a.Mode})
		}
	}
	return s, OK
}

func (n *DefaultFsNode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) RemoveXAttr(attr string, context *Context) Status {
	return ENOSYS
}

func (n *DefaultFsNode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return ENOSYS
}

func (n *DefaultFsNode) ListXAttr(context *Context) (attrs []string, code Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) GetAttr(out *Attr, file File, context *Context) (code Status) {
	if n.Inode().IsDir() {
		out.Mode = S_IFDIR | 0755
	} else {
		out.Mode = S_IFREG | 0644
	}
	return OK
}

func (n *DefaultFsNode) Chmod(file File, perms uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Truncate(file File, size uint64, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Utimens(file File, atime *time.Time, mtime *time.Time, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Fallocate(file File, off uint64, size uint64, mode uint32, context *Context) (code Status) {
	return ENOSYS
}
