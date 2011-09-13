package fuse

import (
	"os"
	"sync"
)

// This is a wrapper that makes a FileSystem threadsafe by
// trivially locking all operations.  For improved performance, you
// should probably invent do your own locking inside the file system.
type LockingFileSystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	FileSystem
	lock sync.Mutex
}

func NewLockingFileSystem(pfs FileSystem) *LockingFileSystem {
	l := new(LockingFileSystem)
	l.FileSystem = pfs
	return l
}

func (me *LockingFileSystem) locked() func() {
	me.lock.Lock()
	return func() { me.lock.Unlock() }
}

func (me *LockingFileSystem) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	defer me.locked()()
	return me.FileSystem.GetAttr(name, context)
}

func (me *LockingFileSystem) Readlink(name string, context *Context) (string, Status) {
	defer me.locked()()
	return me.FileSystem.Readlink(name, context)
}

func (me *LockingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	defer me.locked()()
	return me.FileSystem.Mknod(name, mode, dev, context)
}

func (me *LockingFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	defer me.locked()()
	return me.FileSystem.Mkdir(name, mode, context)
}

func (me *LockingFileSystem) Unlink(name string, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Unlink(name, context)
}

func (me *LockingFileSystem) Rmdir(name string, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Rmdir(name, context)
}

func (me *LockingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Symlink(value, linkName, context)
}

func (me *LockingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Rename(oldName, newName, context)
}

func (me *LockingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Link(oldName, newName, context)
}

func (me *LockingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Chmod(name, mode, context)
}

func (me *LockingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Chown(name, uid, gid, context)
}

func (me *LockingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Truncate(name, offset, context)
}

func (me *LockingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return me.FileSystem.Open(name, flags, context)
}

func (me *LockingFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	defer me.locked()()
	return me.FileSystem.OpenDir(name, context)
}

func (me *LockingFileSystem) OnMount(nodeFs *PathNodeFs) {
	defer me.locked()()
	me.FileSystem.OnMount(nodeFs)
}

func (me *LockingFileSystem) OnUnmount() {
	defer me.locked()()
	me.FileSystem.OnUnmount()
}

func (me *LockingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Access(name, mode, context)
}

func (me *LockingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	defer me.locked()()
	return me.FileSystem.Create(name, flags, mode, context)
}

func (me *LockingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64, context *Context) (code Status) {
	defer me.locked()()
	return me.FileSystem.Utimens(name, AtimeNs, CtimeNs, context)
}

func (me *LockingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	defer me.locked()()
	return me.FileSystem.GetXAttr(name, attr, context)
}

func (me *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	defer me.locked()()
	return me.FileSystem.SetXAttr(name, attr, data, flags, context)
}

func (me *LockingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	defer me.locked()()
	return me.FileSystem.ListXAttr(name, context)
}

func (me *LockingFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	defer me.locked()()
	return me.FileSystem.RemoveXAttr(name, attr, context)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFileSystem struct {
	RawFileSystem
	lock sync.Mutex
}

func (me *LockingRawFileSystem) locked() func() {
	me.lock.Lock()
	return func() { me.lock.Unlock() }
}

func NewLockingRawFileSystem(rfs RawFileSystem) *LockingRawFileSystem {
	l := &LockingRawFileSystem{}
	l.RawFileSystem = rfs
	return l
}

func (me *LockingRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Lookup(h, name)
}

func (me *LockingRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
	defer me.locked()()
	me.RawFileSystem.Forget(h, input)
}

func (me *LockingRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.GetAttr(header, input)
}

func (me *LockingRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.locked()()
	return me.RawFileSystem.Open(header, input)
}

func (me *LockingRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.SetAttr(header, input)
}

func (me *LockingRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Readlink(header)
}

func (me *LockingRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Mknod(header, input, name)
}

func (me *LockingRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Mkdir(header, input, name)
}

func (me *LockingRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Unlink(header, name)
}

func (me *LockingRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Rmdir(header, name)
}

func (me *LockingRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Symlink(header, pointedTo, linkName)
}

func (me *LockingRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Rename(header, input, oldName, newName)
}

func (me *LockingRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Link(header, input, name)
}

func (me *LockingRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	defer me.locked()()
	return me.RawFileSystem.SetXAttr(header, input, attr, data)
}

func (me *LockingRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	defer me.locked()()
	return me.RawFileSystem.GetXAttr(header, attr)
}

func (me *LockingRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	defer me.locked()()
	return me.RawFileSystem.ListXAttr(header)
}

func (me *LockingRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	defer me.locked()()
	return me.RawFileSystem.RemoveXAttr(header, attr)
}

func (me *LockingRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Access(header, input)
}

func (me *LockingRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Create(header, input, name)
}

func (me *LockingRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, h uint64, status Status) {
	defer me.locked()()
	return me.RawFileSystem.OpenDir(header, input)
}

func (me *LockingRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
	defer me.locked()()
	me.RawFileSystem.Release(header, input)
}

func (me *LockingRawFileSystem) ReleaseDir(header *InHeader, h *ReleaseIn) {
	defer me.locked()()
	me.RawFileSystem.ReleaseDir(header, h)
}

func (me *LockingRawFileSystem) Read(header *InHeader, input *ReadIn, bp BufferPool) ([]byte, Status) {
	defer me.locked()()
	return me.RawFileSystem.Read(header, input, bp)
}

func (me *LockingRawFileSystem) Write(header *InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Write(header, input, data)
}

func (me *LockingRawFileSystem) Flush(header *InHeader, input *FlushIn) Status {
	defer me.locked()()
	return me.RawFileSystem.Flush(header, input)
}

func (me *LockingRawFileSystem) Fsync(header *InHeader, input *FsyncIn) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Fsync(header, input)
}

func (me *LockingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	defer me.locked()()
	return me.RawFileSystem.ReadDir(header, input)
}

func (me *LockingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.FsyncDir(header, input)
}
