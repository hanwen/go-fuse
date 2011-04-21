package fuse

import (
	"sync"
)

// This is a wrapper that makes a PathFilesystem threadsafe by
// trivially locking all operations.  For improved performance, you
// should probably invent do your own locking inside the file system.
type LockingPathFilesystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	WrappingPathFilesystem
	lock sync.Mutex
}

func NewLockingPathFilesystem(pfs PathFilesystem) *LockingPathFilesystem {
	l := new(LockingPathFilesystem)
	l.Original = pfs
	return l
}

func (me *LockingPathFilesystem) GetAttr(name string) (*Attr, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetAttr(name)
}

func (me *LockingPathFilesystem) Readlink(name string) (string, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Readlink(name)
}

func (me *LockingPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mknod(name, mode, dev)
}

func (me *LockingPathFilesystem) Mkdir(name string, mode uint32) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mkdir(name, mode)
}

func (me *LockingPathFilesystem) Unlink(name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Unlink(name)
}

func (me *LockingPathFilesystem) Rmdir(name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rmdir(name)
}

func (me *LockingPathFilesystem) Symlink(value string, linkName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Symlink(value, linkName)
}

func (me *LockingPathFilesystem) Rename(oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rename(oldName, newName)
}

func (me *LockingPathFilesystem) Link(oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Link(oldName, newName)
}

func (me *LockingPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Chmod(name, mode)
}

func (me *LockingPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Chown(name, uid, gid)
}

func (me *LockingPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Truncate(name, offset)
}

func (me *LockingPathFilesystem) Open(name string, flags uint32) (file FuseFile, code Status) {
	return me.Original.Open(name, flags)
}

func (me *LockingPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.OpenDir(name)
}

func (me *LockingPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mount(conn)
}

func (me *LockingPathFilesystem) Unmount() {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Unmount()
}

func (me *LockingPathFilesystem) Access(name string, mode uint32) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Access(name, mode)
}

func (me *LockingPathFilesystem) Create(name string, flags uint32, mode uint32) (file FuseFile, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Create(name, flags, mode)
}

func (me *LockingPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Utimens(name, AtimeNs, CtimeNs)
}

func (me *LockingPathFilesystem) GetXAttr(name string, attr string) ([]byte, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetXAttr(name, attr)
}

func (me *LockingPathFilesystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetXAttr(name, attr, data, flags)
}

func (me *LockingPathFilesystem) ListXAttr(name string) ([]string, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ListXAttr(name)
}

func (me *LockingPathFilesystem) RemoveXAttr(name string, attr string) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.RemoveXAttr(name, attr)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFilesystem struct {
	WrappingRawFilesystem
	lock sync.Mutex
}

func NewLockingRawFilesystem(rfs RawFileSystem) *LockingRawFilesystem {
	l := &LockingRawFilesystem{}
	l.Original = rfs
	return l
}

func (me *LockingRawFilesystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Init(h, input)
}

func (me *LockingRawFilesystem) Destroy(h *InHeader, input *InitIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Destroy(h, input)
}

func (me *LockingRawFilesystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Lookup(h, name)
}

func (me *LockingRawFilesystem) Forget(h *InHeader, input *ForgetIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Forget(h, input)
}

func (me *LockingRawFilesystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetAttr(header, input)
}

func (me *LockingRawFilesystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Open(header, input)
}

func (me *LockingRawFilesystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetAttr(header, input)
}

func (me *LockingRawFilesystem) Readlink(header *InHeader) (out []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Readlink(header)
}

func (me *LockingRawFilesystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mknod(header, input, name)
}

func (me *LockingRawFilesystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Mkdir(header, input, name)
}

func (me *LockingRawFilesystem) Unlink(header *InHeader, name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Unlink(header, name)
}

func (me *LockingRawFilesystem) Rmdir(header *InHeader, name string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rmdir(header, name)
}

func (me *LockingRawFilesystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *LockingRawFilesystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *LockingRawFilesystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Link(header, input, name)
}

func (me *LockingRawFilesystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *LockingRawFilesystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.GetXAttr(header, attr)
}

func (me *LockingRawFilesystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ListXAttr(header)
}

func (me *LockingRawFilesystem) RemoveXAttr(header *InHeader, attr string) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.RemoveXAttr(header, attr)
}

func (me *LockingRawFilesystem) Access(header *InHeader, input *AccessIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Access(header, input)
}

func (me *LockingRawFilesystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Create(header, input, name)
}

func (me *LockingRawFilesystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Bmap(header, input)
}

func (me *LockingRawFilesystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Ioctl(header, input)
}

func (me *LockingRawFilesystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Poll(header, input)
}

func (me *LockingRawFilesystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, h uint64, status Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.OpenDir(header, input)
}

func (me *LockingRawFilesystem) Release(header *InHeader, input *ReleaseIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.Release(header, input)
}

func (me *LockingRawFilesystem) ReleaseDir(header *InHeader, h *ReleaseIn) {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.Original.ReleaseDir(header, h)
}

func (me *LockingRawFilesystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Read(input, bp)
}

func (me *LockingRawFilesystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Write(input, data)
}

func (me *LockingRawFilesystem) Flush(input *FlushIn) Status {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Flush(input)
}

func (me *LockingRawFilesystem) Fsync(input *FsyncIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.Fsync(input)
}

func (me *LockingRawFilesystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.ReadDir(header, input)
}

func (me *LockingRawFilesystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	me.lock.Lock()
	defer me.lock.Unlock()
	return me.Original.FsyncDir(header, input)
}
