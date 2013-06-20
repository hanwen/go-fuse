package pathfs

import (
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// This is a wrapper that makes a FileSystem threadsafe by
// trivially locking all operations.  For improved performance, you
// should probably invent do your own locking inside the file system.
type LockingFileSystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	FS   FileSystem
	lock sync.Mutex
}

var _ = ((FileSystem)((*LockingFileSystem)(nil)))

func NewLockingFileSystem(pfs FileSystem) *LockingFileSystem {
	l := new(LockingFileSystem)
	l.FS = pfs
	return l
}

func (fs *LockingFileSystem) String() string {
	defer fs.locked()()
	return fs.FS.String()
}

func (fs *LockingFileSystem) StatFs(name string) *fuse.StatfsOut {
	defer fs.locked()()
	return fs.FS.StatFs(name)
}

func (fs *LockingFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *LockingFileSystem) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	defer fs.locked()()
	return fs.FS.GetAttr(name, context)
}

func (fs *LockingFileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	defer fs.locked()()
	return fs.FS.Readlink(name, context)
}

func (fs *LockingFileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status {
	defer fs.locked()()
	return fs.FS.Mknod(name, mode, dev, context)
}

func (fs *LockingFileSystem) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	defer fs.locked()()
	return fs.FS.Mkdir(name, mode, context)
}

func (fs *LockingFileSystem) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Unlink(name, context)
}

func (fs *LockingFileSystem) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Rmdir(name, context)
}

func (fs *LockingFileSystem) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Symlink(value, linkName, context)
}

func (fs *LockingFileSystem) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Rename(oldName, newName, context)
}

func (fs *LockingFileSystem) Link(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Link(oldName, newName, context)
}

func (fs *LockingFileSystem) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Chmod(name, mode, context)
}

func (fs *LockingFileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Chown(name, uid, gid, context)
}

func (fs *LockingFileSystem) Truncate(name string, offset uint64, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Truncate(name, offset, context)
}

func (fs *LockingFileSystem) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	return fs.FS.Open(name, flags, context)
}

func (fs *LockingFileSystem) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, status fuse.Status) {
	defer fs.locked()()
	return fs.FS.OpenDir(name, context)
}

func (fs *LockingFileSystem) OnMount(nodeFs *PathNodeFs) {
	defer fs.locked()()
	fs.FS.OnMount(nodeFs)
}

func (fs *LockingFileSystem) OnUnmount() {
	defer fs.locked()()
	fs.FS.OnUnmount()
}

func (fs *LockingFileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Access(name, mode, context)
}

func (fs *LockingFileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Create(name, flags, mode, context)
}

func (fs *LockingFileSystem) Utimens(name string, Atime *time.Time, Mtime *time.Time, context *fuse.Context) (code fuse.Status) {
	defer fs.locked()()
	return fs.FS.Utimens(name, Atime, Mtime, context)
}

func (fs *LockingFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	defer fs.locked()()
	return fs.FS.GetXAttr(name, attr, context)
}

func (fs *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	defer fs.locked()()
	return fs.FS.SetXAttr(name, attr, data, flags, context)
}

func (fs *LockingFileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	defer fs.locked()()
	return fs.FS.ListXAttr(name, context)
}

func (fs *LockingFileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	defer fs.locked()()
	return fs.FS.RemoveXAttr(name, attr, context)
}
