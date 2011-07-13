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

func (me *LockingFileSystem) GetAttr(name string) (*os.FileInfo, Status) {
	defer me.locked()()
	return me.FileSystem.GetAttr(name)
}

func (me *LockingFileSystem) Readlink(name string) (string, Status) {
	defer me.locked()()
	return me.FileSystem.Readlink(name)
}

func (me *LockingFileSystem) Mknod(name string, mode uint32, dev uint32) Status {
	defer me.locked()()
	return me.FileSystem.Mknod(name, mode, dev)
}

func (me *LockingFileSystem) Mkdir(name string, mode uint32) Status {
	defer me.locked()()
	return me.FileSystem.Mkdir(name, mode)
}

func (me *LockingFileSystem) Unlink(name string) (code Status) {
	defer me.locked()()
	return me.FileSystem.Unlink(name)
}

func (me *LockingFileSystem) Rmdir(name string) (code Status) {
	defer me.locked()()
	return me.FileSystem.Rmdir(name)
}

func (me *LockingFileSystem) Symlink(value string, linkName string) (code Status) {
	defer me.locked()()
	return me.FileSystem.Symlink(value, linkName)
}

func (me *LockingFileSystem) Rename(oldName string, newName string) (code Status) {
	defer me.locked()()
	return me.FileSystem.Rename(oldName, newName)
}

func (me *LockingFileSystem) Link(oldName string, newName string) (code Status) {
	defer me.locked()()
	return me.FileSystem.Link(oldName, newName)
}

func (me *LockingFileSystem) Chmod(name string, mode uint32) (code Status) {
	defer me.locked()()
	return me.FileSystem.Chmod(name, mode)
}

func (me *LockingFileSystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	defer me.locked()()
	return me.FileSystem.Chown(name, uid, gid)
}

func (me *LockingFileSystem) Truncate(name string, offset uint64) (code Status) {
	defer me.locked()()
	return me.FileSystem.Truncate(name, offset)
}

func (me *LockingFileSystem) Open(name string, flags uint32) (file File, code Status) {
	return me.FileSystem.Open(name, flags)
}

func (me *LockingFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	defer me.locked()()
	return me.FileSystem.OpenDir(name)
}

func (me *LockingFileSystem) Mount(conn *FileSystemConnector) {
	defer me.locked()()
	me.FileSystem.Mount(conn)
}

func (me *LockingFileSystem) Unmount() {
	defer me.locked()()
	me.FileSystem.Unmount()
}

func (me *LockingFileSystem) Access(name string, mode uint32) (code Status) {
	defer me.locked()()
	return me.FileSystem.Access(name, mode)
}

func (me *LockingFileSystem) Create(name string, flags uint32, mode uint32) (file File, code Status) {
	defer me.locked()()
	return me.FileSystem.Create(name, flags, mode)
}

func (me *LockingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	defer me.locked()()
	return me.FileSystem.Utimens(name, AtimeNs, CtimeNs)
}

func (me *LockingFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	defer me.locked()()
	return me.FileSystem.GetXAttr(name, attr)
}

func (me *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	defer me.locked()()
	return me.FileSystem.SetXAttr(name, attr, data, flags)
}

func (me *LockingFileSystem) ListXAttr(name string) ([]string, Status) {
	defer me.locked()()
	return me.FileSystem.ListXAttr(name)
}

func (me *LockingFileSystem) RemoveXAttr(name string, attr string) Status {
	defer me.locked()()
	return me.FileSystem.RemoveXAttr(name, attr)
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

func (me *LockingRawFileSystem) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	defer me.locked()()
	return me.RawFileSystem.Read(input, bp)
}

func (me *LockingRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	defer me.locked()()
	return me.RawFileSystem.Write(input, data)
}

func (me *LockingRawFileSystem) Flush(input *FlushIn) Status {
	defer me.locked()()
	return me.RawFileSystem.Flush(input)
}

func (me *LockingRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.Fsync(input)
}

func (me *LockingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	defer me.locked()()
	return me.RawFileSystem.ReadDir(header, input)
}

func (me *LockingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	defer me.locked()()
	return me.RawFileSystem.FsyncDir(header, input)
}
