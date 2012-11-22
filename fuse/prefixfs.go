package fuse

import (
	"fmt"
	"path/filepath"
)

// PrefixFileSystem adds a path prefix to incoming calls.
type PrefixFileSystem struct {
	FileSystem
	Prefix string
}

func (fs *PrefixFileSystem) prefixed(n string) string {
	return filepath.Join(fs.Prefix, n)
}

func (fs *PrefixFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	return fs.FileSystem.GetAttr(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) Readlink(name string, context *Context) (string, Status) {
	return fs.FileSystem.Readlink(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	return fs.FileSystem.Mknod(fs.prefixed(name), mode, dev, context)
}

func (fs *PrefixFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	return fs.FileSystem.Mkdir(fs.prefixed(name), mode, context)
}

func (fs *PrefixFileSystem) Unlink(name string, context *Context) (code Status) {
	return fs.FileSystem.Unlink(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) Rmdir(name string, context *Context) (code Status) {
	return fs.FileSystem.Rmdir(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	return fs.FileSystem.Symlink(value, fs.prefixed(linkName), context)
}

func (fs *PrefixFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	return fs.FileSystem.Rename(fs.prefixed(oldName), fs.prefixed(newName), context)
}

func (fs *PrefixFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	return fs.FileSystem.Link(fs.prefixed(oldName), fs.prefixed(newName), context)
}

func (fs *PrefixFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	return fs.FileSystem.Chmod(fs.prefixed(name), mode, context)
}

func (fs *PrefixFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	return fs.FileSystem.Chown(fs.prefixed(name), uid, gid, context)
}

func (fs *PrefixFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	return fs.FileSystem.Truncate(fs.prefixed(name), offset, context)
}

func (fs *PrefixFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return fs.FileSystem.Open(fs.prefixed(name), flags, context)
}

func (fs *PrefixFileSystem) OpenDir(name string, context *Context) (stream []DirEntry, status Status) {
	return fs.FileSystem.OpenDir(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) OnMount(nodeFs *PathNodeFs) {
	fs.FileSystem.OnMount(nodeFs)
}

func (fs *PrefixFileSystem) OnUnmount() {
	fs.FileSystem.OnUnmount()
}

func (fs *PrefixFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return fs.FileSystem.Access(fs.prefixed(name), mode, context)
}

func (fs *PrefixFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	return fs.FileSystem.Create(fs.prefixed(name), flags, mode, context)
}

func (fs *PrefixFileSystem) Utimens(name string, AtimeNs int64, CtimeNs int64, context *Context) (code Status) {
	return fs.FileSystem.Utimens(fs.prefixed(name), AtimeNs, CtimeNs, context)
}

func (fs *PrefixFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return fs.FileSystem.GetXAttr(fs.prefixed(name), attr, context)
}

func (fs *PrefixFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	return fs.FileSystem.SetXAttr(fs.prefixed(name), attr, data, flags, context)
}

func (fs *PrefixFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	return fs.FileSystem.ListXAttr(fs.prefixed(name), context)
}

func (fs *PrefixFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return fs.FileSystem.RemoveXAttr(fs.prefixed(name), attr, context)
}

func (fs *PrefixFileSystem) String() string {
	return fmt.Sprintf("PrefixFileSystem(%s,%s)", fs.FileSystem.String(), fs.Prefix)
}
