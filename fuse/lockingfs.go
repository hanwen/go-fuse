package fuse

import (
	"sync"

	"github.com/hanwen/go-fuse/raw"
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

func (fs *LockingFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *LockingFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	defer fs.locked()()
	return fs.FileSystem.GetAttr(name, context)
}

func (fs *LockingFileSystem) Readlink(name string, context *Context) (string, Status) {
	defer fs.locked()()
	return fs.FileSystem.Readlink(name, context)
}

func (fs *LockingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	defer fs.locked()()
	return fs.FileSystem.Mknod(name, mode, dev, context)
}

func (fs *LockingFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	defer fs.locked()()
	return fs.FileSystem.Mkdir(name, mode, context)
}

func (fs *LockingFileSystem) Unlink(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Unlink(name, context)
}

func (fs *LockingFileSystem) Rmdir(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Rmdir(name, context)
}

func (fs *LockingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Symlink(value, linkName, context)
}

func (fs *LockingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Rename(oldName, newName, context)
}

func (fs *LockingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Link(oldName, newName, context)
}

func (fs *LockingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Chmod(name, mode, context)
}

func (fs *LockingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Chown(name, uid, gid, context)
}

func (fs *LockingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Truncate(name, offset, context)
}

func (fs *LockingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return fs.FileSystem.Open(name, flags, context)
}

func (fs *LockingFileSystem) OpenDir(name string, context *Context) (stream []DirEntry, status Status) {
	defer fs.locked()()
	return fs.FileSystem.OpenDir(name, context)
}

func (fs *LockingFileSystem) OnMount(nodeFs *PathNodeFs) {
	defer fs.locked()()
	fs.FileSystem.OnMount(nodeFs)
}

func (fs *LockingFileSystem) OnUnmount() {
	defer fs.locked()()
	fs.FileSystem.OnUnmount()
}

func (fs *LockingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Access(name, mode, context)
}

func (fs *LockingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	defer fs.locked()()
	return fs.FileSystem.Create(name, flags, mode, context)
}

func (fs *LockingFileSystem) Utimens(name string, AtimeNs int64, CtimeNs int64, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FileSystem.Utimens(name, AtimeNs, CtimeNs, context)
}

func (fs *LockingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	defer fs.locked()()
	return fs.FileSystem.GetXAttr(name, attr, context)
}

func (fs *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	defer fs.locked()()
	return fs.FileSystem.SetXAttr(name, attr, data, flags, context)
}

func (fs *LockingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	defer fs.locked()()
	return fs.FileSystem.ListXAttr(name, context)
}

func (fs *LockingFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	defer fs.locked()()
	return fs.FileSystem.RemoveXAttr(name, attr, context)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFileSystem struct {
	RawFileSystem
	lock sync.Mutex
}

func (fs *LockingRawFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func NewLockingRawFileSystem(rfs RawFileSystem) *LockingRawFileSystem {
	l := &LockingRawFileSystem{}
	l.RawFileSystem = rfs
	return l
}

func (fs *LockingRawFileSystem) Lookup(out *raw.EntryOut, h *raw.InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Lookup(out, h, name)
}

func (fs *LockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.RawFileSystem.Forget(nodeID, nlookup)
}

func (fs *LockingRawFileSystem) GetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.GetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.GetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Open(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status) {

	defer fs.locked()()
	return fs.RawFileSystem.Open(out, header, input)
}

func (fs *LockingRawFileSystem) SetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.SetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.SetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Readlink(header *raw.InHeader) (out []byte, code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Readlink(header)
}

func (fs *LockingRawFileSystem) Mknod(out *raw.EntryOut, header *raw.InHeader, input *raw.MknodIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Mknod(out, header, input, name)
}

func (fs *LockingRawFileSystem) Mkdir(out *raw.EntryOut, header *raw.InHeader, input *raw.MkdirIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Mkdir(out, header, input, name)
}

func (fs *LockingRawFileSystem) Unlink(header *raw.InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Unlink(header, name)
}

func (fs *LockingRawFileSystem) Rmdir(header *raw.InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Rmdir(header, name)
}

func (fs *LockingRawFileSystem) Symlink(out *raw.EntryOut, header *raw.InHeader, pointedTo string, linkName string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Symlink(out, header, pointedTo, linkName)
}

func (fs *LockingRawFileSystem) Rename(header *raw.InHeader, input *raw.RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Rename(header, input, oldName, newName)
}

func (fs *LockingRawFileSystem) Link(out *raw.EntryOut, header *raw.InHeader, input *raw.LinkIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Link(out, header, input, name)
}

func (fs *LockingRawFileSystem) SetXAttr(header *raw.InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.RawFileSystem.SetXAttr(header, input, attr, data)
}

func (fs *LockingRawFileSystem) GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.GetXAttrData(header, attr)
}

func (fs *LockingRawFileSystem) GetXAttrSize(header *raw.InHeader, attr string) (sz int, code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.GetXAttrSize(header, attr)
}

func (fs *LockingRawFileSystem) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.ListXAttr(header)
}

func (fs *LockingRawFileSystem) RemoveXAttr(header *raw.InHeader, attr string) Status {
	defer fs.locked()()
	return fs.RawFileSystem.RemoveXAttr(header, attr)
}

func (fs *LockingRawFileSystem) Access(header *raw.InHeader, input *raw.AccessIn) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Access(header, input)
}

func (fs *LockingRawFileSystem) Create(out *raw.CreateOut, header *raw.InHeader, input *raw.CreateIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Create(out, header, input, name)
}

func (fs *LockingRawFileSystem) OpenDir(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status) {
	defer fs.locked()()
	return fs.RawFileSystem.OpenDir(out, header, input)
}

func (fs *LockingRawFileSystem) Release(header *raw.InHeader, input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFileSystem.Release(header, input)
}

func (fs *LockingRawFileSystem) ReleaseDir(header *raw.InHeader, h *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFileSystem.ReleaseDir(header, h)
}

func (fs *LockingRawFileSystem) Read(header *raw.InHeader, input *ReadIn, bp BufferPool) ([]byte, Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Read(header, input, bp)
}

func (fs *LockingRawFileSystem) Write(header *raw.InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Write(header, input, data)
}

func (fs *LockingRawFileSystem) Flush(header *raw.InHeader, input *raw.FlushIn) Status {
	defer fs.locked()()
	return fs.RawFileSystem.Flush(header, input)
}

func (fs *LockingRawFileSystem) Fsync(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.Fsync(header, input)
}

func (fs *LockingRawFileSystem) ReadDir(out *DirEntryList, header *raw.InHeader, input *ReadIn) (Status) {
	defer fs.locked()()
	return fs.RawFileSystem.ReadDir(out, header, input)
}

func (fs *LockingRawFileSystem) FsyncDir(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFileSystem.FsyncDir(header, input)
}
