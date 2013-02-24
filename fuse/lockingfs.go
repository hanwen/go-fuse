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
	FS FileSystem
	lock sync.Mutex
}

var _ = ((FileSystem)((*LockingFileSystem)(nil)))

func NewLockingFileSystem(pfs FileSystem) *LockingFileSystem {
	l := new(LockingFileSystem)
	l.FS = pfs
	return l
}

func (fs *LockingFileSystem) String() string {
	defer fs.locked()()
	return fs.FS.String()
}

func (fs *LockingFileSystem) StatFs(name string) *StatfsOut {
	defer fs.locked()()
	return fs.FS.StatFs(name)
}

func (fs *LockingFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *LockingFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	defer fs.locked()()
	return fs.FS.GetAttr(name, context)
}

func (fs *LockingFileSystem) Readlink(name string, context *Context) (string, Status) {
	defer fs.locked()()
	return fs.FS.Readlink(name, context)
}

func (fs *LockingFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	defer fs.locked()()
	return fs.FS.Mknod(name, mode, dev, context)
}

func (fs *LockingFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	defer fs.locked()()
	return fs.FS.Mkdir(name, mode, context)
}

func (fs *LockingFileSystem) Unlink(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Unlink(name, context)
}

func (fs *LockingFileSystem) Rmdir(name string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Rmdir(name, context)
}

func (fs *LockingFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Symlink(value, linkName, context)
}

func (fs *LockingFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Rename(oldName, newName, context)
}

func (fs *LockingFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Link(oldName, newName, context)
}

func (fs *LockingFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Chmod(name, mode, context)
}

func (fs *LockingFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Chown(name, uid, gid, context)
}

func (fs *LockingFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Truncate(name, offset, context)
}

func (fs *LockingFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return fs.FS.Open(name, flags, context)
}

func (fs *LockingFileSystem) OpenDir(name string, context *Context) (stream []DirEntry, status Status) {
	defer fs.locked()()
	return fs.FS.OpenDir(name, context)
}

func (fs *LockingFileSystem) OnMount(nodeFs *PathNodeFs) {
	defer fs.locked()()
	fs.FS.OnMount(nodeFs)
}

func (fs *LockingFileSystem) OnUnmount() {
	defer fs.locked()()
	fs.FS.OnUnmount()
}

func (fs *LockingFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Access(name, mode, context)
}

func (fs *LockingFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	defer fs.locked()()
	return fs.FS.Create(name, flags, mode, context)
}

func (fs *LockingFileSystem) Utimens(name string, Atime *time.Time, Mtime *time.Time, context *Context) (code Status) {
	defer fs.locked()()
	return fs.FS.Utimens(name, Atime, Mtime, context)
}

func (fs *LockingFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	defer fs.locked()()
	return fs.FS.GetXAttr(name, attr, context)
}

func (fs *LockingFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	defer fs.locked()()
	return fs.FS.SetXAttr(name, attr, data, flags, context)
}

func (fs *LockingFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	defer fs.locked()()
	return fs.FS.ListXAttr(name, context)
}

func (fs *LockingFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	defer fs.locked()()
	return fs.FS.RemoveXAttr(name, attr, context)
}

////////////////////////////////////////////////////////////////
// Locking raw FS.

type LockingRawFileSystem struct {
	RawFS RawFileSystem
	lock sync.Mutex
}

var _ = (RawFileSystem)((*LockingRawFileSystem)(nil))

func (fs *LockingRawFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func NewLockingRawFileSystem(rfs RawFileSystem) *LockingRawFileSystem {
	l := &LockingRawFileSystem{}
	l.RawFS = rfs
	return l
}

func (fs *LockingRawFileSystem) Lookup(out *raw.EntryOut, h *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Lookup(out, h, name)
}

func (fs *LockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.RawFS.Forget(nodeID, nlookup)
}

func (fs *LockingRawFileSystem) GetAttr(out *raw.AttrOut, header *Context, input *raw.GetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.GetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Open(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {

	defer fs.locked()()
	return fs.RawFS.Open(out, header, input)
}

func (fs *LockingRawFileSystem) SetAttr(out *raw.AttrOut, header *Context, input *raw.SetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetAttr(out, header, input)
}

func (fs *LockingRawFileSystem) Readlink(header *Context) (out []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.Readlink(header)
}

func (fs *LockingRawFileSystem) Mknod(out *raw.EntryOut, header *Context, input *raw.MknodIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mknod(out, header, input, name)
}

func (fs *LockingRawFileSystem) Mkdir(out *raw.EntryOut, header *Context, input *raw.MkdirIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mkdir(out, header, input, name)
}

func (fs *LockingRawFileSystem) Unlink(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Unlink(header, name)
}

func (fs *LockingRawFileSystem) Rmdir(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rmdir(header, name)
}

func (fs *LockingRawFileSystem) Symlink(out *raw.EntryOut, header *Context, pointedTo string, linkName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Symlink(out, header, pointedTo, linkName)
}

func (fs *LockingRawFileSystem) Rename(header *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rename(header, input, oldName, newName)
}

func (fs *LockingRawFileSystem) Link(out *raw.EntryOut, header *Context, input *raw.LinkIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Link(out, header, input, name)
}

func (fs *LockingRawFileSystem) SetXAttr(header *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.RawFS.SetXAttr(header, input, attr, data)
}

func (fs *LockingRawFileSystem) GetXAttrData(header *Context, attr string) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrData(header, attr)
}

func (fs *LockingRawFileSystem) GetXAttrSize(header *Context, attr string) (sz int, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrSize(header, attr)
}

func (fs *LockingRawFileSystem) ListXAttr(header *Context) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.ListXAttr(header)
}

func (fs *LockingRawFileSystem) RemoveXAttr(header *Context, attr string) Status {
	defer fs.locked()()
	return fs.RawFS.RemoveXAttr(header, attr)
}

func (fs *LockingRawFileSystem) Access(header *Context, input *raw.AccessIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Access(header, input)
}

func (fs *LockingRawFileSystem) Create(out *raw.CreateOut, header *Context, input *raw.CreateIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Create(out, header, input, name)
}

func (fs *LockingRawFileSystem) OpenDir(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {
	defer fs.locked()()
	return fs.RawFS.OpenDir(out, header, input)
}

func (fs *LockingRawFileSystem) Release(header *Context, input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.Release(header, input)
}

func (fs *LockingRawFileSystem) ReleaseDir(header *Context, h *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.ReleaseDir(header, h)
}

func (fs *LockingRawFileSystem) Read(header *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	defer fs.locked()()
	return fs.RawFS.Read(header, input, buf)
}

func (fs *LockingRawFileSystem) Write(header *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.RawFS.Write(header, input, data)
}

func (fs *LockingRawFileSystem) Flush(header *Context, input *raw.FlushIn) Status {
	defer fs.locked()()
	return fs.RawFS.Flush(header, input)
}

func (fs *LockingRawFileSystem) Fsync(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fsync(header, input)
}

func (fs *LockingRawFileSystem) ReadDir(out *DirEntryList, header *Context, input *raw.ReadIn) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDir(out, header, input)
}

func (fs *LockingRawFileSystem) FsyncDir(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.FsyncDir(header, input)
}

func (fs *LockingRawFileSystem) Init(params *RawFsInit) {
	defer fs.locked()()
	fs.RawFS.Init(params)
}

func (fs *LockingRawFileSystem) StatFs(out *StatfsOut, context *Context) (code Status) {
	defer fs.locked()()
	return fs.RawFS.StatFs(out, context)
}

func (fs *LockingRawFileSystem) Fallocate(c *Context, in *raw.FallocateIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fallocate(c, in)
}

func (fs *LockingRawFileSystem) String() string {
	defer fs.locked()()
	return fmt.Sprintf("Locked(%s)", fs.RawFS.String())
}
