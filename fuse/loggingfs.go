package fuse

import (
	"log"
	"os"
	"fmt"
)

var _ = log.Print
var _ = fmt.Print

// LoggingFileSystem is a wrapper that prints out what a FileSystem is doing.
type LoggingFileSystem struct {
	FileSystem
}

func NewLoggingFileSystem(fs FileSystem) *LoggingFileSystem {
	t := new(LoggingFileSystem)
	t.FileSystem = fs
	return t
}

func (me *LoggingFileSystem) Print(name string, arg string) {
	log.Printf("Op %s arg %s", name, arg)
}

func (me *LoggingFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	me.Print("GetAttr", name)
	return me.FileSystem.GetAttr(name, context)
}

func (me *LoggingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	me.Print("GetXAttr", name)
	return me.FileSystem.GetXAttr(name, attr, context)
}

func (me *LoggingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	me.Print("SetXAttr", name)
	return me.FileSystem.SetXAttr(name, attr, data, flags, context)
}

func (me *LoggingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	me.Print("ListXAttr", name)
	return me.FileSystem.ListXAttr(name, context)
}

func (me *LoggingFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	me.Print("RemoveXAttr", name)
	return me.FileSystem.RemoveXAttr(name, attr, context)
}

func (me *LoggingFileSystem) Readlink(name string, context *Context) (string, Status) {
	me.Print("Readlink", name)
	return me.FileSystem.Readlink(name, context)
}

func (me *LoggingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	me.Print("Mknod", name)
	return me.FileSystem.Mknod(name, mode, dev, context)
}

func (me *LoggingFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	me.Print("Mkdir", name)
	return me.FileSystem.Mkdir(name, mode, context)
}

func (me *LoggingFileSystem) Unlink(name string, context *Context) (code Status) {
	me.Print("Unlink", name)
	return me.FileSystem.Unlink(name, context)
}

func (me *LoggingFileSystem) Rmdir(name string, context *Context) (code Status) {
	me.Print("Rmdir", name)
	return me.FileSystem.Rmdir(name, context)
}

func (me *LoggingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	me.Print("Symlink", linkName)
	return me.FileSystem.Symlink(value, linkName, context)
}

func (me *LoggingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	me.Print("Rename", oldName)
	return me.FileSystem.Rename(oldName, newName, context)
}

func (me *LoggingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	me.Print("Link", newName)
	return me.FileSystem.Link(oldName, newName, context)
}

func (me *LoggingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	me.Print("Chmod", name)
	return me.FileSystem.Chmod(name, mode, context)
}

func (me *LoggingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	me.Print("Chown", name)
	return me.FileSystem.Chown(name, uid, gid, context)
}

func (me *LoggingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	me.Print("Truncate", name)
	return me.FileSystem.Truncate(name, offset, context)
}

func (me *LoggingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	me.Print("Open", name)
	return me.FileSystem.Open(name, flags, context)
}

func (me *LoggingFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	me.Print("OpenDir", name)
	return me.FileSystem.OpenDir(name, context)
}

func (me *LoggingFileSystem) Mount(conn *FileSystemConnector) {
	me.Print("Mount", "")
	me.FileSystem.Mount(conn)
}

func (me *LoggingFileSystem) Unmount() {
	me.Print("Unmount", "")
	me.FileSystem.Unmount()
}

func (me *LoggingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	me.Print("Access", name)
	return me.FileSystem.Access(name, mode, context)
}

func (me *LoggingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	me.Print("Create", name)
	return me.FileSystem.Create(name, flags, mode, context)
}

func (me *LoggingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	me.Print("Utimens", name)
	return me.FileSystem.Utimens(name, AtimeNs, CtimeNs, context)
}
