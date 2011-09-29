package fuse

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var _ = log.Println

type MemNodeFs struct {
	DefaultNodeFileSystem
	backingStorePrefix string
	root               *memNode

	mutex    sync.Mutex
	nextFree int
}

func (me *MemNodeFs) String() string {
	return fmt.Sprintf("MemNodeFs(%s)", me.backingStorePrefix)
}

func (me *MemNodeFs) Root() FsNode {
	return me.root
}

func (me *MemNodeFs) newNode() *memNode {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	n := &memNode{
		fs: me,
		id: me.nextFree,
	}
	now := time.Nanoseconds()
	n.info.Mtime_ns = now
	n.info.Atime_ns = now
	n.info.Ctime_ns = now
	n.info.Mode = S_IFDIR | 0777
	me.nextFree++
	return n
}

func NewMemNodeFs(prefix string) *MemNodeFs {
	me := &MemNodeFs{}
	me.backingStorePrefix = prefix
	me.root = me.newNode()
	return me
}

func (me *MemNodeFs) Filename(n *Inode) string {
	mn := n.FsNode().(*memNode)
	return mn.filename()
}

type memNode struct {
	DefaultFsNode
	fs *MemNodeFs
	id int

	link string
	info os.FileInfo
}

func (me *memNode) newNode(isdir bool) *memNode {
	n := me.fs.newNode()
	me.Inode().New(isdir, n)
	return n
}

func (me *memNode) filename() string {
	return fmt.Sprintf("%s%d", me.fs.backingStorePrefix, me.id)
}

func (me *memNode) Deletable() bool {
	return false
}

func (me *memNode) Readlink(c *Context) ([]byte, Status) {
	return []byte(me.link), OK
}

func (me *memNode) Mkdir(name string, mode uint32, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	n := me.newNode(true)
	n.info.Mode = mode | S_IFDIR
	me.Inode().AddChild(name, n.Inode())
	return &n.info, n, OK
}

func (me *memNode) Unlink(name string, context *Context) (code Status) {
	ch := me.Inode().RmChild(name)
	if ch == nil {
		return ENOENT
	}
	return OK
}

func (me *memNode) Rmdir(name string, context *Context) (code Status) {
	return me.Unlink(name, context)
}

func (me *memNode) Symlink(name string, content string, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	n := me.newNode(false)
	n.info.Mode = S_IFLNK | 0777
	n.link = content
	me.Inode().AddChild(name, n.Inode())

	return &n.info, n, OK
}

func (me *memNode) Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status) {
	ch := me.Inode().RmChild(oldName)
	newParent.Inode().RmChild(newName)
	newParent.Inode().AddChild(newName, ch)
	return OK
}

func (me *memNode) Link(name string, existing FsNode, context *Context) (fi *os.FileInfo, newNode FsNode, code Status) {
	me.Inode().AddChild(name, existing.Inode())
	fi, code = existing.GetAttr(nil, context)
	return fi, existing, code
}

func (me *memNode) Create(name string, flags uint32, mode uint32, context *Context) (file File, fi *os.FileInfo, newNode FsNode, code Status) {
	n := me.newNode(false)
	n.info.Mode = mode | S_IFREG

	f, err := os.Create(n.filename())
	if err != nil {
		return nil, nil, nil, OsErrorToErrno(err)
	}
	me.Inode().AddChild(name, n.Inode())
	return n.newFile(f), &n.info, n, OK
}

type memNodeFile struct {
	LoopbackFile
	node *memNode
}

func (me *memNodeFile) String() string {
	return fmt.Sprintf("memNodeFile(%s)", me.LoopbackFile.String())
}

func (me *memNodeFile) InnerFile() File {
	return &me.LoopbackFile
}

func (me *memNodeFile) Flush() Status {
	code := me.LoopbackFile.Flush()
	fi, _ := me.LoopbackFile.GetAttr()
	me.node.info.Size = fi.Size
	me.node.info.Blocks = fi.Blocks
	return code
}

func (me *memNode) newFile(f *os.File) File {
	return &memNodeFile{
		LoopbackFile: LoopbackFile{File: f},
		node:         me,
	}
}

func (me *memNode) Open(flags uint32, context *Context) (file File, code Status) {
	f, err := os.OpenFile(me.filename(), int(flags), 0666)
	if err != nil {
		return nil, OsErrorToErrno(err)
	}

	return me.newFile(f), OK
}

func (me *memNode) GetAttr(file File, context *Context) (fi *os.FileInfo, code Status) {
	return &me.info, OK
}

func (me *memNode) Truncate(file File, size uint64, context *Context) (code Status) {
	if file != nil {
		return file.Truncate(size)
	}

	me.info.Size = int64(size)
	err := os.Truncate(me.filename(), int64(size))
	me.info.Ctime_ns = time.Nanoseconds()
	return OsErrorToErrno(err)
}

func (me *memNode) Utimens(file File, atime uint64, mtime uint64, context *Context) (code Status) {
	me.info.Atime_ns = int64(atime)
	me.info.Mtime_ns = int64(mtime)
	me.info.Ctime_ns = time.Nanoseconds()
	return OK
}

func (me *memNode) Chmod(file File, perms uint32, context *Context) (code Status) {
	me.info.Mode = (me.info.Mode ^ 07777) | perms
	me.info.Ctime_ns = time.Nanoseconds()
	return OK
}

func (me *memNode) Chown(file File, uid uint32, gid uint32, context *Context) (code Status) {
	me.info.Uid = int(uid)
	me.info.Gid = int(gid)
	me.info.Ctime_ns = time.Nanoseconds()
	return OK
}
