package pathfs

import (
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

var _ = FileSystem((*DefaultFileSystem)(nil))

// DefaultFileSystem implements a FileSystem that returns ENOSYS for every operation.
type DefaultFileSystem struct{}

// DefaultFileSystem
func (fs *DefaultFileSystem) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	return "", fuse.ENOSYS
}

func (fs *DefaultFileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Link(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Truncate(name string, offset uint64, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, status fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) OnMount(nodeFs *PathNodeFs) {
}

func (fs *DefaultFileSystem) OnUnmount() {
}

func (fs *DefaultFileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DefaultFileSystem) Utimens(name string, Atime *time.Time, Mtime *time.Time, context *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (fs *DefaultFileSystem) String() string {
	return "DefaultFileSystem"
}

func (fs *DefaultFileSystem) StatFs(name string) *fuse.StatfsOut {
	return nil
}
