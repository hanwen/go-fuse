package pathfs

import (
	"fmt"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// This is a wrapper that only exposes read-only operations.
type ReadonlyFileSystem struct {
	FileSystem
}

var _ = (FileSystem)((*ReadonlyFileSystem)(nil))

func (fs *ReadonlyFileSystem) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	return fs.FileSystem.GetAttr(name, context)
}

func (fs *ReadonlyFileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	return fs.FileSystem.Readlink(name, context)
}

func (fs *ReadonlyFileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Symlink(value string, linkName string, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Link(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Truncate(name string, offset uint64, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) Open(name string, flags uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}
	file, code = fs.FileSystem.Open(name, flags, context)
	return &fuse.ReadOnlyFile{file}, code
}

func (fs *ReadonlyFileSystem) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, status fuse.Status) {
	return fs.FileSystem.OpenDir(name, context)
}

func (fs *ReadonlyFileSystem) OnMount(nodeFs *PathNodeFs) {
	fs.FileSystem.OnMount(nodeFs)
}

func (fs *ReadonlyFileSystem) OnUnmount() {
	fs.FileSystem.OnUnmount()
}

func (fs *ReadonlyFileSystem) String() string {
	return fmt.Sprintf("ReadonlyFileSystem(%v)", fs.FileSystem)
}

func (fs *ReadonlyFileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fs.FileSystem.Access(name, mode, context)
}

func (fs *ReadonlyFileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file fuse.File, code fuse.Status) {
	return nil, fuse.EPERM
}

func (fs *ReadonlyFileSystem) Utimens(name string, atime *time.Time, ctime *time.Time, context *fuse.Context) (code fuse.Status) {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	return fs.FileSystem.GetXAttr(name, attr, context)
}

func (fs *ReadonlyFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	return fuse.EPERM
}

func (fs *ReadonlyFileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	return fs.FileSystem.ListXAttr(name, context)
}

func (fs *ReadonlyFileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	return fuse.EPERM
}
