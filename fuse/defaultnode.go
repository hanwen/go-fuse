package fuse

import (
	"log"
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

func (n *DefaultFsNode) StatFs() *StatfsOut {
	return nil
}

func (n *DefaultFsNode) SetInode(node *Inode) {
	if n.inode != nil {
		panic("already have Inode")
	}
	if node == nil {
		panic("SetInode called with nil Inode.")
	}
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

func (n *DefaultFsNode) Lookup(name string, context *Context) (fi *Attr, node FsNode, code Status) {
	return nil, nil, ENOENT
}

func (n *DefaultFsNode) Access(mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Readlink(c *Context) ([]byte, Status) {
	return nil, ENOSYS
}

func (n *DefaultFsNode) Mknod(name string, mode uint32, dev uint32, context *Context) (fi *Attr, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}
func (n *DefaultFsNode) Mkdir(name string, mode uint32, context *Context) (fi *Attr, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}
func (n *DefaultFsNode) Unlink(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *DefaultFsNode) Rmdir(name string, context *Context) (code Status) {
	return ENOSYS
}
func (n *DefaultFsNode) Symlink(name string, content string, context *Context) (fi *Attr, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (n *DefaultFsNode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (n *DefaultFsNode) Link(name string, existing FsNode, context *Context) (fi *Attr, newNode FsNode, code Status) {
	return nil, nil, ENOSYS
}

func (n *DefaultFsNode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *Attr, newNode FsNode, code Status) {
	return nil, nil, nil, ENOSYS
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
		fi, code := child.FsNode().GetAttr(nil, context)
		if code.Ok() {
			s = append(s, DirEntry{Name: name, Mode: fi.Mode})
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

func (n *DefaultFsNode) GetAttr(file File, context *Context) (fi *Attr, code Status) {
	if n.Inode().IsDir() {
		return &Attr{Mode: S_IFDIR | 0755}, OK
	}
	return &Attr{Mode: S_IFREG | 0644}, OK
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

func (n *DefaultFsNode) Utimens(file File, atime int64, mtime int64, context *Context) (code Status) {
	return ENOSYS
}
