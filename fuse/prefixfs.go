package fuse

import (
	"fmt"
	"os"
	"path/filepath"
)

// PrefixFileSystem adds a path prefix to incoming calls. 
type PrefixFileSystem struct {
	FileSystem
	Prefix string
}

func (me *PrefixFileSystem) prefixed(n string) string {
	return filepath.Join(me.Prefix, n)
}

func (me *PrefixFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	return me.FileSystem.GetAttr(me.prefixed(name), context)
}

func (me *PrefixFileSystem) Readlink(name string, context *Context) (string, Status) {
	return me.FileSystem.Readlink(me.prefixed(name), context)
}

func (me *PrefixFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	return me.FileSystem.Mknod(me.prefixed(name), mode, dev, context)
}

func (me *PrefixFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	return me.FileSystem.Mkdir(me.prefixed(name), mode, context)
}

func (me *PrefixFileSystem) Unlink(name string, context *Context) (code Status) {
	return me.FileSystem.Unlink(me.prefixed(name), context)
}

func (me *PrefixFileSystem) Rmdir(name string, context *Context) (code Status) {
	return me.FileSystem.Rmdir(me.prefixed(name), context)
}

func (me *PrefixFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	return me.FileSystem.Symlink(value, me.prefixed(linkName), context)
}

func (me *PrefixFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	return me.FileSystem.Rename(me.prefixed(oldName), me.prefixed(newName), context)
}

func (me *PrefixFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	return me.FileSystem.Link(me.prefixed(oldName), me.prefixed(newName), context)
}

func (me *PrefixFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	return me.FileSystem.Chmod(me.prefixed(name), mode, context)
}

func (me *PrefixFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	return me.FileSystem.Chown(me.prefixed(name), uid, gid, context)
}

func (me *PrefixFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	return me.FileSystem.Truncate(me.prefixed(name), offset, context)
}

func (me *PrefixFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return me.FileSystem.Open(me.prefixed(name), flags, context)
}

func (me *PrefixFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	return me.FileSystem.OpenDir(me.prefixed(name), context)
}

func (me *PrefixFileSystem) OnMount(nodeFs *PathNodeFs) {
	me.FileSystem.OnMount(nodeFs)
}

func (me *PrefixFileSystem) OnUnmount() {
	me.FileSystem.OnUnmount()
}

func (me *PrefixFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return me.FileSystem.Access(me.prefixed(name), mode, context)
}

func (me *PrefixFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	return me.FileSystem.Create(me.prefixed(name), flags, mode, context)
}

func (me *PrefixFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	return me.FileSystem.Utimens(me.prefixed(name), AtimeNs, CtimeNs, context)
}

func (me *PrefixFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return me.FileSystem.GetXAttr(me.prefixed(name), attr, context)
}

func (me *PrefixFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	return me.FileSystem.SetXAttr(me.prefixed(name), attr, data, flags, context)
}

func (me *PrefixFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	return me.FileSystem.ListXAttr(me.prefixed(name), context)
}

func (me *PrefixFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return me.FileSystem.RemoveXAttr(me.prefixed(name), attr, context)
}

func (me *PrefixFileSystem) String() string {
	return fmt.Sprintf("PrefixFileSystem(%s,%s)", me.FileSystem.String(), me.Prefix)
}
