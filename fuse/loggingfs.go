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

func (me *LoggingFileSystem) GetAttr(name string) (*os.FileInfo, Status) {
	me.Print("GetAttr", name)
	return me.FileSystem.GetAttr(name)
}

func (me *LoggingFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	me.Print("GetXAttr", name)
	return me.FileSystem.GetXAttr(name, attr)
}

func (me *LoggingFileSystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	me.Print("SetXAttr", name)
	return me.FileSystem.SetXAttr(name, attr, data, flags)
}

func (me *LoggingFileSystem) ListXAttr(name string) ([]string, Status) {
	me.Print("ListXAttr", name)
	return me.FileSystem.ListXAttr(name)
}

func (me *LoggingFileSystem) RemoveXAttr(name string, attr string) Status {
	me.Print("RemoveXAttr", name)
	return me.FileSystem.RemoveXAttr(name, attr)
}

func (me *LoggingFileSystem) Readlink(name string) (string, Status) {
	me.Print("Readlink", name)
	return me.FileSystem.Readlink(name)
}

func (me *LoggingFileSystem) Mknod(name string, mode uint32, dev uint32) Status {
	me.Print("Mknod", name)
	return me.FileSystem.Mknod(name, mode, dev)
}

func (me *LoggingFileSystem) Mkdir(name string, mode uint32) Status {
	me.Print("Mkdir", name)
	return me.FileSystem.Mkdir(name, mode)
}

func (me *LoggingFileSystem) Unlink(name string) (code Status) {
	me.Print("Unlink", name)
	return me.FileSystem.Unlink(name)
}

func (me *LoggingFileSystem) Rmdir(name string) (code Status) {
	me.Print("Rmdir", name)
	return me.FileSystem.Rmdir(name)
}

func (me *LoggingFileSystem) Symlink(value string, linkName string) (code Status) {
	me.Print("Symlink", linkName)
	return me.FileSystem.Symlink(value, linkName)
}

func (me *LoggingFileSystem) Rename(oldName string, newName string) (code Status) {
	me.Print("Rename", oldName)
	return me.FileSystem.Rename(oldName, newName)
}

func (me *LoggingFileSystem) Link(oldName string, newName string) (code Status) {
	me.Print("Link", newName)
	return me.FileSystem.Link(oldName, newName)
}

func (me *LoggingFileSystem) Chmod(name string, mode uint32) (code Status) {
	me.Print("Chmod", name)
	return me.FileSystem.Chmod(name, mode)
}

func (me *LoggingFileSystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	me.Print("Chown", name)
	return me.FileSystem.Chown(name, uid, gid)
}

func (me *LoggingFileSystem) Truncate(name string, offset uint64) (code Status) {
	me.Print("Truncate", name)
	return me.FileSystem.Truncate(name, offset)
}

func (me *LoggingFileSystem) Open(name string, flags uint32) (file File, code Status) {
	me.Print("Open", name)
	return me.FileSystem.Open(name, flags)
}

func (me *LoggingFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	me.Print("OpenDir", name)
	return me.FileSystem.OpenDir(name)
}

func (me *LoggingFileSystem) Mount(conn *FileSystemConnector) {
	me.Print("Mount", "")
	me.FileSystem.Mount(conn)
}

func (me *LoggingFileSystem) Unmount() {
	me.Print("Unmount", "")
	me.FileSystem.Unmount()
}

func (me *LoggingFileSystem) Access(name string, mode uint32) (code Status) {
	me.Print("Access", name)
	return me.FileSystem.Access(name, mode)
}

func (me *LoggingFileSystem) Create(name string, flags uint32, mode uint32) (file File, code Status) {
	me.Print("Create", name)
	return me.FileSystem.Create(name, flags, mode)
}

func (me *LoggingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	me.Print("Utimens", name)
	return me.FileSystem.Utimens(name, AtimeNs, CtimeNs)
}
