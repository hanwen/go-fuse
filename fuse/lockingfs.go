package fuse

import (
	"fmt"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

// This is a wrapper that makes a FileSystem threadsafe by
// trivially locking all operations.  For improved performance, you
// should probably invent do your own locking inside the file system.
type LockingFileSystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	fs FileSystem
	lock sync.Mutex
}

var _ = ((FileSystem)((*LockingFileSystem)(nil)))

func NewLockingFileSystem(pfs FileSystem) *LockingFileSystem {
	l := new(LockingFileSystem)
	l.fs = pfs
	return l
}

func (fs *LockingFileSystem) String() string {
	defer fs.locked()()
	return fs.fs.String()
}

func (fs *LockingFileSystem) StatFs(name string) *StatfsOut {
	defer fs.locked()()
	return fs.fs.StatFs(name)
}

func (fs *LockingFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *LockingFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	defer fs.locked()()
	return fs.fs.GetAttr(name, context)
}

func (fs *LockingFileSystem) Readlink(name string, context *Context) (string, Status) {
	defer fs.locked()()
	return fs.fs.Readlink(name, context)
}

func (fs *LockingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	defer fs.locked()()
	return fs.fs.Mknod(name, mode, dev, context)
}

func (fs *LockingFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	defer fs.locked()()
	return fs.fs.Mkdir(name, mode, context)
}

func (fs *LockingFileSystem) Unlink(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Unlink(name, context)
}

func (fs *LockingFileSystem) Rmdir(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Rmdir(name, context)
}

func (fs *LockingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Symlink(value, linkName, context)
}

func (fs *LockingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Rename(oldName, newName, context)
}

func (fs *LockingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Link(oldName, newName, context)
}

func (fs *LockingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Chmod(name, mode, context)
}

func (fs *LockingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Chown(name, uid, gid, context)
}

func (fs *LockingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Truncate(name, offset, context)
}

func (fs *LockingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return fs.fs.Open(name, flags, context)
}

func (fs *LockingFileSystem) OpenDir(name string, context *Context) (stream []DirEntry, status Status) {
	defer fs.locked()()
	return fs.fs.OpenDir(name, context)
}

func (fs *LockingFileSystem) OnMount(nodeFs *PathNodeFs) {
	defer fs.locked()()
	fs.fs.OnMount(nodeFs)
}

func (fs *LockingFileSystem) OnUnmount() {
	defer fs.locked()()
	fs.fs.OnUnmount()
}

func (fs *LockingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Access(name, mode, context)
}

func (fs *LockingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	defer fs.locked()()
	return fs.fs.Create(name, flags, mode, context)
}

func (fs *LockingFileSystem) Utimens(name string, Atime *time.Time, Mtime *time.Time, context *Context) (code Status) {
	defer fs.locked()()
	return fs.fs.Utimens(name, Atime, Mtime, context)
}

func (fs *LockingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	defer fs.locked()()
	return fs.fs.GetXAttr(name, attr, context)
}

func (fs *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	defer fs.locked()()
	return fs.fs.SetXAttr(name, attr, data, flags, context)
}

func (fs *LockingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	defer fs.locked()()
	return fs.fs.ListXAttr(name, context)
}

func (fs *LockingFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	defer fs.locked()()
	return fs.fs.RemoveXAttr(name, attr, context)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFileSystem struct {
	raw RawFileSystem
	lock sync.Mutex
}

var _ = (RawFileSystem)((*LockingRawFileSystem)(nil))

func (fs *LockingRawFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func NewLockingRawFileSystem(rfs RawFileSystem) *LockingRawFileSystem {
	l := &LockingRawFileSystem{}
	l.raw = rfs
	return l
}

func (fs *LockingRawFileSystem) Lookup(out *raw.EntryOut, h *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Lookup(out, h, name)
}

func (fs *LockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.raw.Forget(nodeID, nlookup)
}

func (fs *LockingRawFileSystem) GetAttr(out *raw.AttrOut, header *Context, input *raw.GetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.raw.GetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Open(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {

	defer fs.locked()()
	return fs.raw.Open(out, header, input)
}

func (fs *LockingRawFileSystem) SetAttr(out *raw.AttrOut, header *Context, input *raw.SetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.raw.SetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Readlink(header *Context) (out []byte, code Status) {
	defer fs.locked()()
	return fs.raw.Readlink(header)
}

func (fs *LockingRawFileSystem) Mknod(out *raw.EntryOut, header *Context, input *raw.MknodIn, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Mknod(out, header, input, name)
}

func (fs *LockingRawFileSystem) Mkdir(out *raw.EntryOut, header *Context, input *raw.MkdirIn, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Mkdir(out, header, input, name)
}

func (fs *LockingRawFileSystem) Unlink(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Unlink(header, name)
}

func (fs *LockingRawFileSystem) Rmdir(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Rmdir(header, name)
}

func (fs *LockingRawFileSystem) Symlink(out *raw.EntryOut, header *Context, pointedTo string, linkName string) (code Status) {
	defer fs.locked()()
	return fs.raw.Symlink(out, header, pointedTo, linkName)
}

func (fs *LockingRawFileSystem) Rename(header *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.raw.Rename(header, input, oldName, newName)
}

func (fs *LockingRawFileSystem) Link(out *raw.EntryOut, header *Context, input *raw.LinkIn, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Link(out, header, input, name)
}

func (fs *LockingRawFileSystem) SetXAttr(header *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.raw.SetXAttr(header, input, attr, data)
}

func (fs *LockingRawFileSystem) GetXAttrData(header *Context, attr string) (data []byte, code Status) {
	defer fs.locked()()
	return fs.raw.GetXAttrData(header, attr)
}

func (fs *LockingRawFileSystem) GetXAttrSize(header *Context, attr string) (sz int, code Status) {
	defer fs.locked()()
	return fs.raw.GetXAttrSize(header, attr)
}

func (fs *LockingRawFileSystem) ListXAttr(header *Context) (data []byte, code Status) {
	defer fs.locked()()
	return fs.raw.ListXAttr(header)
}

func (fs *LockingRawFileSystem) RemoveXAttr(header *Context, attr string) Status {
	defer fs.locked()()
	return fs.raw.RemoveXAttr(header, attr)
}

func (fs *LockingRawFileSystem) Access(header *Context, input *raw.AccessIn) (code Status) {
	defer fs.locked()()
	return fs.raw.Access(header, input)
}

func (fs *LockingRawFileSystem) Create(out *raw.CreateOut, header *Context, input *raw.CreateIn, name string) (code Status) {
	defer fs.locked()()
	return fs.raw.Create(out, header, input, name)
}

func (fs *LockingRawFileSystem) OpenDir(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {
	defer fs.locked()()
	return fs.raw.OpenDir(out, header, input)
}

func (fs *LockingRawFileSystem) Release(header *Context, input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.raw.Release(header, input)
}

func (fs *LockingRawFileSystem) ReleaseDir(header *Context, h *raw.ReleaseIn) {
	defer fs.locked()()
	fs.raw.ReleaseDir(header, h)
}

func (fs *LockingRawFileSystem) Read(header *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	defer fs.locked()()
	return fs.raw.Read(header, input, buf)
}

func (fs *LockingRawFileSystem) Write(header *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.raw.Write(header, input, data)
}

func (fs *LockingRawFileSystem) Flush(header *Context, input *raw.FlushIn) Status {
	defer fs.locked()()
	return fs.raw.Flush(header, input)
}

func (fs *LockingRawFileSystem) Fsync(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.raw.Fsync(header, input)
}

func (fs *LockingRawFileSystem) ReadDir(out *DirEntryList, header *Context, input *raw.ReadIn) Status {
	defer fs.locked()()
	return fs.raw.ReadDir(out, header, input)
}

func (fs *LockingRawFileSystem) FsyncDir(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.raw.FsyncDir(header, input)
}

func (fs *LockingRawFileSystem) Init(params *RawFsInit) {
	defer fs.locked()()
	fs.raw.Init(params)
}

func (fs *LockingRawFileSystem) StatFs(out *StatfsOut, context *Context) (code Status) {
	defer fs.locked()()
	return fs.raw.StatFs(out, context)
}

func (fs *LockingRawFileSystem) String() string {
	defer fs.locked()()
	return fmt.Sprintf("Locked(%s)", fs.raw.String())
}
