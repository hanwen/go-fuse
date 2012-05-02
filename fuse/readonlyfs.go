package fuse

import (
	"fmt"
)

// This is a wrapper that only exposes read-only operations.
type ReadonlyFileSystem struct {
	FileSystem
}

func (fs *ReadonlyFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	return fs.FileSystem.GetAttr(name, context)
}

func (fs *ReadonlyFileSystem) Readlink(name string, context *Context) (string, Status) {
	return fs.FileSystem.Readlink(name, context)
}

func (fs *ReadonlyFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	return EPERM
}

func (fs *ReadonlyFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	return EPERM
}

func (fs *ReadonlyFileSystem) Unlink(name string, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Rmdir(name string, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	if flags&O_ANYWRITE != 0 {
		return nil, EPERM
	}
	file, code = fs.FileSystem.Open(name, flags, context)
	return &ReadOnlyFile{file}, code
}

func (fs *ReadonlyFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
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

func (fs *ReadonlyFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return fs.FileSystem.Access(name, mode, context)
}

func (fs *ReadonlyFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	return nil, EPERM
}

func (fs *ReadonlyFileSystem) Utimens(name string, AtimeNs int64, CtimeNs int64, context *Context) (code Status) {
	return EPERM
}

func (fs *ReadonlyFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return fs.FileSystem.GetXAttr(name, attr, context)
}

func (fs *ReadonlyFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	return EPERM
}

func (fs *ReadonlyFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	return fs.FileSystem.ListXAttr(name, context)
}

func (fs *ReadonlyFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return EPERM
}
