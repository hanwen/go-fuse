package fuse

import (
	"sync"
)

// This is a wrapper that makes a PathFileSystem threadsafe by
// trivially locking all operations.  For improved performance, you
// should probably invent do your own locking inside the file system.
type LockingPathFileSystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	WrappingPathFileSystem
	lock sync.Mutex
}

func NewLockingPathFileSystem(pfs PathFileSystem) *LockingPathFileSystem {
	l := new(LockingPathFileSystem)
	l.Original = pfs
	return l
}

func (me *LockingPathFileSystem) GetAttr(name string) (*Attr, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetAttr(name)
}

func (me *LockingPathFileSystem) Readlink(name string) (string, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Readlink(name)
}

func (me *LockingPathFileSystem) Mknod(name string, mode uint32, dev uint32) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mknod(name, mode, dev)
}

func (me *LockingPathFileSystem) Mkdir(name string, mode uint32) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mkdir(name, mode)
}

func (me *LockingPathFileSystem) Unlink(name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Unlink(name)
}

func (me *LockingPathFileSystem) Rmdir(name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rmdir(name)
}

func (me *LockingPathFileSystem) Symlink(value string, linkName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Symlink(value, linkName)
}

func (me *LockingPathFileSystem) Rename(oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rename(oldName, newName)
}

func (me *LockingPathFileSystem) Link(oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Link(oldName, newName)
}

func (me *LockingPathFileSystem) Chmod(name string, mode uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Chmod(name, mode)
}

func (me *LockingPathFileSystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Chown(name, uid, gid)
}

func (me *LockingPathFileSystem) Truncate(name string, offset uint64) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Truncate(name, offset)
}

func (me *LockingPathFileSystem) Open(name string, flags uint32) (file FuseFile, code Status) {
	return me.Original.Open(name, flags)
}

func (me *LockingPathFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.OpenDir(name)
}

func (me *LockingPathFileSystem) Mount(conn *PathFileSystemConnector) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mount(conn)
}

func (me *LockingPathFileSystem) Unmount() {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Unmount()
}

func (me *LockingPathFileSystem) Access(name string, mode uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Access(name, mode)
}

func (me *LockingPathFileSystem) Create(name string, flags uint32, mode uint32) (file FuseFile, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Create(name, flags, mode)
}

func (me *LockingPathFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Utimens(name, AtimeNs, CtimeNs)
}

func (me *LockingPathFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetXAttr(name, attr)
}

func (me *LockingPathFileSystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetXAttr(name, attr, data, flags)
}

func (me *LockingPathFileSystem) ListXAttr(name string) ([]string, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ListXAttr(name)
}

func (me *LockingPathFileSystem) RemoveXAttr(name string, attr string) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.RemoveXAttr(name, attr)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFileSystem struct {
	WrappingRawFileSystem
	lock sync.Mutex
}

func NewLockingRawFileSystem(rfs RawFileSystem) *LockingRawFileSystem {
	l := &LockingRawFileSystem{}
	l.Original = rfs
	return l
}

func (me *LockingRawFileSystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Init(h, input)
}

func (me *LockingRawFileSystem) Destroy(h *InHeader, input *InitIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Destroy(h, input)
}

func (me *LockingRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Lookup(h, name)
}

func (me *LockingRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Forget(h, input)
}

func (me *LockingRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetAttr(header, input)
}

func (me *LockingRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Open(header, input)
}

func (me *LockingRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetAttr(header, input)
}

func (me *LockingRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Readlink(header)
}

func (me *LockingRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mknod(header, input, name)
}

func (me *LockingRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mkdir(header, input, name)
}

func (me *LockingRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Unlink(header, name)
}

func (me *LockingRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rmdir(header, name)
}

func (me *LockingRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *LockingRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *LockingRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Link(header, input, name)
}

func (me *LockingRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *LockingRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetXAttr(header, attr)
}

func (me *LockingRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ListXAttr(header)
}

func (me *LockingRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.RemoveXAttr(header, attr)
}

func (me *LockingRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Access(header, input)
}

func (me *LockingRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Create(header, input, name)
}

func (me *LockingRawFileSystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Bmap(header, input)
}

func (me *LockingRawFileSystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Ioctl(header, input)
}

func (me *LockingRawFileSystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Poll(header, input)
}

func (me *LockingRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, h uint64, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.OpenDir(header, input)
}

func (me *LockingRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Release(header, input)
}

func (me *LockingRawFileSystem) ReleaseDir(header *InHeader, h *ReleaseIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.ReleaseDir(header, h)
}

func (me *LockingRawFileSystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Read(input, bp)
}

func (me *LockingRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Write(input, data)
}

func (me *LockingRawFileSystem) Flush(input *FlushIn) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Flush(input)
}

func (me *LockingRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Fsync(input)
}

func (me *LockingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ReadDir(header, input)
}

func (me *LockingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.FsyncDir(header, input)
}
