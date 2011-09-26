package fuse

import (
	"fmt"
	"os"
)

// This is a wrapper that only exposes read-only operations.
type ReadonlyFileSystem struct {
	FileSystem
}

func (me *ReadonlyFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	return me.FileSystem.GetAttr(name, context)
}

func (me *ReadonlyFileSystem) Readlink(name string, context *Context) (string, Status) {
	return me.FileSystem.Readlink(name, context)
}

func (me *ReadonlyFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	return EPERM
}

func (me *ReadonlyFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	return EPERM
}

func (me *ReadonlyFileSystem) Unlink(name string, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Rmdir(name string, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	if flags&O_ANYWRITE != 0 {
		return nil, EPERM
	}
	file, code = me.FileSystem.Open(name, flags, context)
	return &ReadOnlyFile{file}, code
}

func (me *ReadonlyFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	return me.FileSystem.OpenDir(name, context)
}

func (me *ReadonlyFileSystem) OnMount(nodeFs *PathNodeFs) {
	me.FileSystem.OnMount(nodeFs)
}

func (me *ReadonlyFileSystem) OnUnmount() {
	me.FileSystem.OnUnmount()
}

func (me *ReadonlyFileSystem) String() string {
	return fmt.Sprintf("ReadonlyFileSystem(%v)", me.FileSystem)
}

func (me *ReadonlyFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return me.FileSystem.Access(name, mode, context)
}

func (me *ReadonlyFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	return nil, EPERM
}

func (me *ReadonlyFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	return EPERM
}

func (me *ReadonlyFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return me.FileSystem.GetXAttr(name, attr, context)
}

func (me *ReadonlyFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	return EPERM
}

func (me *ReadonlyFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	return me.FileSystem.ListXAttr(name, context)
}

func (me *ReadonlyFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return EPERM
}
