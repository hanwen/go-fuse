package fuse

import (
	"time"
	"log"
	"os"
	"fmt"
)

var _ = log.Print
var _ = fmt.Print

// TimingFileSystem is a wrapper to collect timings for a FileSystem
type TimingFileSystem struct {
	FileSystem

	*LatencyMap
}

func NewTimingFileSystem(fs FileSystem) *TimingFileSystem {
	t := new(TimingFileSystem)
	t.LatencyMap = NewLatencyMap()
	t.FileSystem = fs
	return t
}

func (me *TimingFileSystem) startTimer(name string, arg string) (closure func()) {
	start := time.Nanoseconds()

	return func() {
		dt := (time.Nanoseconds() - start) / 1e6
		me.LatencyMap.Add(name, arg, dt)
	}
}

func (me *TimingFileSystem) OperationCounts() map[string]int {
	return me.LatencyMap.Counts()
}

func (me *TimingFileSystem) Latencies() map[string]float64 {
	return me.LatencyMap.Latencies(1e-3)
}

func (me *TimingFileSystem) HotPaths(operation string) (paths []string) {
	return me.LatencyMap.TopArgs(operation)
}

func (me *TimingFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	defer me.startTimer("GetAttr", name)()
	return me.FileSystem.GetAttr(name, context)
}

func (me *TimingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	defer me.startTimer("GetXAttr", name)()
	return me.FileSystem.GetXAttr(name, attr, context)
}

func (me *TimingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) (code Status) {
	defer me.startTimer("SetXAttr", name)()
	return me.FileSystem.SetXAttr(name, attr, data, flags, context)
}

func (me *TimingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	defer me.startTimer("ListXAttr", name)()
	return me.FileSystem.ListXAttr(name, context)
}

func (me *TimingFileSystem) RemoveXAttr(name string, attr string, context *Context) (code Status) {
	defer me.startTimer("RemoveXAttr", name)()
	return me.FileSystem.RemoveXAttr(name, attr, context)
}

func (me *TimingFileSystem) Readlink(name string, context *Context) (string, Status) {
	defer me.startTimer("Readlink", name)()
	return me.FileSystem.Readlink(name, context)
}

func (me *TimingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) (code Status) {
	defer me.startTimer("Mknod", name)()
	return me.FileSystem.Mknod(name, mode, dev, context)
}

func (me *TimingFileSystem) Mkdir(name string, mode uint32, context *Context) (code Status) {
	defer me.startTimer("Mkdir", name)()
	return me.FileSystem.Mkdir(name, mode, context)
}

func (me *TimingFileSystem) Unlink(name string, context *Context) (code Status) {
	defer me.startTimer("Unlink", name)()
	return me.FileSystem.Unlink(name, context)
}

func (me *TimingFileSystem) Rmdir(name string, context *Context) (code Status) {
	defer me.startTimer("Rmdir", name)()
	return me.FileSystem.Rmdir(name, context)
}

func (me *TimingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	defer me.startTimer("Symlink", linkName)()
	return me.FileSystem.Symlink(value, linkName, context)
}

func (me *TimingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	defer me.startTimer("Rename", oldName)()
	return me.FileSystem.Rename(oldName, newName, context)
}

func (me *TimingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	defer me.startTimer("Link", newName)()
	return me.FileSystem.Link(oldName, newName, context)
}

func (me *TimingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	defer me.startTimer("Chmod", name)()
	return me.FileSystem.Chmod(name, mode, context)
}

func (me *TimingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	defer me.startTimer("Chown", name)()
	return me.FileSystem.Chown(name, uid, gid, context)
}

func (me *TimingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	defer me.startTimer("Truncate", name)()
	return me.FileSystem.Truncate(name, offset, context)
}

func (me *TimingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	defer me.startTimer("Open", name)()
	return me.FileSystem.Open(name, flags, context)
}

func (me *TimingFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	defer me.startTimer("OpenDir", name)()
	return me.FileSystem.OpenDir(name, context)
}

func (me *TimingFileSystem) Mount(nodeFs *PathNodeFs, conn *FileSystemConnector) {
	defer me.startTimer("Mount", "")()
	me.FileSystem.Mount(nodeFs, conn)
}

func (me *TimingFileSystem) Unmount() {
	defer me.startTimer("Unmount", "")()
	me.FileSystem.Unmount()
}

func (me *TimingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	defer me.startTimer("Access", name)()
	return me.FileSystem.Access(name, mode, context)
}

func (me *TimingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	defer me.startTimer("Create", name)()
	return me.FileSystem.Create(name, flags, mode, context)
}

func (me *TimingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	defer me.startTimer("Utimens", name)()
	return me.FileSystem.Utimens(name, AtimeNs, CtimeNs, context)
}
