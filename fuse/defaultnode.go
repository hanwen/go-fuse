package fuse
import (
	"os"
)

type DefaultNodeFileSystem struct {
	root DefaultFsNode
}

func (me *DefaultNodeFileSystem) Unmount() {
}

func (me *DefaultNodeFileSystem) Mount(conn *FileSystemConnector) {
}

func (me *DefaultNodeFileSystem) StatFs() *StatfsOut {
	return nil
}
func (me *DefaultNodeFileSystem) Root() FsNode {
	return &me.root
}

////////////////////////////////////////////////////////////////
// FsNode default

type DefaultFsNode struct {
	inode *Inode
}

func (me *DefaultFsNode) SetInode(node *Inode) {
	if me.inode != nil {
		panic("already have Inode")
	}
	me.inode = node
}

func (me *DefaultFsNode) Inode() *Inode {
	return me.inode
}

func (me *DefaultFsNode) RmChild(name string, child FsNode) {
}

func (me *DefaultFsNode) AddChild(name string, child FsNode) {
}


func (me *DefaultFsNode) Lookup(name string) (fi *os.FileInfo, node FsNode, code Status)  {
	return nil, nil, ENOSYS
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

func (me *DefaultFsNode) Open(flags uint32, context *Context) (file File, code Status)  {
	return nil, ENOSYS
}

func (me *DefaultFsNode) Flush(file File, openFlags uint32, context *Context) (code Status)  {
	return ENOSYS
}

func (me *DefaultFsNode) OpenDir(context *Context) (chan DirEntry, Status)  {
	return nil, ENOSYS
}

func (me *DefaultFsNode) GetXAttr(attribute string, context *Context) (data []byte, code Status)  {
	return nil, ENOSYS
}

func (me *DefaultFsNode) RemoveXAttr(attr string, context *Context) Status  {
	return ENOSYS
}

func (me *DefaultFsNode) SetXAttr(attr string, data []byte, flags int, context *Context) Status  {
	return ENOSYS
}

func (me *DefaultFsNode) ListXAttr(context *Context) (attrs []string, code Status)  {
	return nil, ENOSYS
}


func (me *DefaultFsNode) GetAttr(file File, context *Context) (fi *os.FileInfo, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFsNode) Chmod(file File, perms uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	return ENOSYS
}

func (me *DefaultFsNode) Truncate(file File, size uint64, context *Context) (code Status)  {
	return ENOSYS
}

func (me *DefaultFsNode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	return ENOSYS
}

