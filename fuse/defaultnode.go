package fuse

import (
	"time"
)

// NewDefaultNodeFileSystem returns a dummy implementation of
// NodeFileSystem, for embedding in structs.
func NewDefaultNodeFileSystem() NodeFileSystem {
	return (*defaultNodeFileSystem)(nil)
}

type defaultNodeFileSystem struct {
}

func (fs *defaultNodeFileSystem) OnUnmount() {
}

func (fs *defaultNodeFileSystem) OnMount(conn *FileSystemConnector) {

}

func (fs *defaultNodeFileSystem) Root() FsNode {
	return NewDefaultFsNode()
}

func (fs *defaultNodeFileSystem) String() string {
	return "defaultNodeFileSystem"
}

func (fs *defaultNodeFileSystem) SetDebug(dbg bool) {
}

// NewDefaultFsNode returns an implementation of FsNode that returns
// ENOSYS for all operations.
func NewDefaultFsNode() FsNode {
	return &defaultFsNode{}
}

type defaultFsNode struct {
	inode *Inode
}

func (n *defaultFsNode) StatFs() *StatfsOut {
	return nil
}

func (n *defaultFsNode) SetInode(node *Inode) {
	n.inode = node
}

func (n *defaultFsNode) Deletable() bool {
	return true
}

func (n *defaultFsNode) Inode() *Inode {
	return n.inode
}

func (n *defaultFsNode) OnForget() {
}

func (n *defaultFsNode) Lookup(out *Attr, name string, context *Context) (node FsNode, code Status) {
	return nil, ENOENT
}

func (n *defaultFsNode) Access(mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Readlink(c *Context) ([]byte, Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) Mknod(name string, mode uint32, dev uint32, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}
func (n *defaultFsNode) Mkdir(name string, mode uint32, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}
func (n *defaultFsNode) Unlink(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *defaultFsNode) Rmdir(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *defaultFsNode) Symlink(name string, content string, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Link(name string, existing FsNode, context *Context) (newNode FsNode, code Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) Create(name string, flags uint32, mode uint32, context *Context) (file File, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (n *defaultFsNode) Open(flags uint32, context *Context) (file File, code Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) Flush(file File, openFlags uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) OpenDir(context *Context) ([]DirEntry, Status) {
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

func (n *defaultFsNode) GetXAttr(attribute string, context *Context) (data []byte, code Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) RemoveXAttr(attr string, context *Context) Status {
	return ENOSYS
}

func (n *defaultFsNode) SetXAttr(attr string, data []byte, flags int, context *Context) Status {
	return ENOSYS
}

func (n *defaultFsNode) ListXAttr(context *Context) (attrs []string, code Status) {
	return nil, ENOSYS
}

func (n *defaultFsNode) GetAttr(out *Attr, file File, context *Context) (code Status) {
	if n.Inode().IsDir() {
		out.Mode = S_IFDIR | 0755
	} else {
		out.Mode = S_IFREG | 0644
	}
	return OK
}

func (n *defaultFsNode) Chmod(file File, perms uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Truncate(file File, size uint64, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Utimens(file File, atime *time.Time, mtime *time.Time, context *Context) (code Status) {
	return ENOSYS
}

func (n *defaultFsNode) Fallocate(file File, off uint64, size uint64, mode uint32, context *Context) (code Status) {
	return ENOSYS
}
