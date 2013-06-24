package fuse

import (
	"fmt"
	"sync"

	"github.com/hanwen/go-fuse/raw"
)

////////////////////////////////////////////////////////////////
// Locking raw FS.

type lockingRawFileSystem struct {
	RawFS RawFileSystem
	lock  sync.Mutex
}

var _ = (RawFileSystem)((*lockingRawFileSystem)(nil))

// Returns a Wrap
func NewLockingRawFileSystem(fs RawFileSystem) RawFileSystem {
	return &lockingRawFileSystem{
		RawFS: fs,
	}
}

func (fs *lockingRawFileSystem) FS() RawFileSystem {
	return fs.RawFS
}

func (fs *lockingRawFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *lockingRawFileSystem) Lookup(out *raw.EntryOut, h *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Lookup(out, h, name)
}

func (fs *lockingRawFileSystem) SetDebug(dbg bool) {
	defer fs.locked()()
	fs.RawFS.SetDebug(dbg)
}

func (fs *lockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.RawFS.Forget(nodeID, nlookup)
}

func (fs *lockingRawFileSystem) GetAttr(out *raw.AttrOut, header *Context, input *raw.GetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.GetAttr(out, header, input)
}

func (fs *lockingRawFileSystem) Open(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {

	defer fs.locked()()
	return fs.RawFS.Open(out, header, input)
}

func (fs *lockingRawFileSystem) SetAttr(out *raw.AttrOut, header *Context, input *raw.SetAttrIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetAttr(out, header, input)
}

func (fs *lockingRawFileSystem) Readlink(header *Context) (out []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.Readlink(header)
}

func (fs *lockingRawFileSystem) Mknod(out *raw.EntryOut, header *Context, input *raw.MknodIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mknod(out, header, input, name)
}

func (fs *lockingRawFileSystem) Mkdir(out *raw.EntryOut, header *Context, input *raw.MkdirIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mkdir(out, header, input, name)
}

func (fs *lockingRawFileSystem) Unlink(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Unlink(header, name)
}

func (fs *lockingRawFileSystem) Rmdir(header *Context, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rmdir(header, name)
}

func (fs *lockingRawFileSystem) Symlink(out *raw.EntryOut, header *Context, pointedTo string, linkName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Symlink(out, header, pointedTo, linkName)
}

func (fs *lockingRawFileSystem) Rename(header *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rename(header, input, oldName, newName)
}

func (fs *lockingRawFileSystem) Link(out *raw.EntryOut, header *Context, input *raw.LinkIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Link(out, header, input, name)
}

func (fs *lockingRawFileSystem) SetXAttr(header *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.RawFS.SetXAttr(header, input, attr, data)
}

func (fs *lockingRawFileSystem) GetXAttrData(header *Context, attr string) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrData(header, attr)
}

func (fs *lockingRawFileSystem) GetXAttrSize(header *Context, attr string) (sz int, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrSize(header, attr)
}

func (fs *lockingRawFileSystem) ListXAttr(header *Context) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.ListXAttr(header)
}

func (fs *lockingRawFileSystem) RemoveXAttr(header *Context, attr string) Status {
	defer fs.locked()()
	return fs.RawFS.RemoveXAttr(header, attr)
}

func (fs *lockingRawFileSystem) Access(header *Context, input *raw.AccessIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Access(header, input)
}

func (fs *lockingRawFileSystem) Create(out *raw.CreateOut, header *Context, input *raw.CreateIn, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Create(out, header, input, name)
}

func (fs *lockingRawFileSystem) OpenDir(out *raw.OpenOut, header *Context, input *raw.OpenIn) (status Status) {
	defer fs.locked()()
	return fs.RawFS.OpenDir(out, header, input)
}

func (fs *lockingRawFileSystem) Release(header *Context, input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.Release(header, input)
}

func (fs *lockingRawFileSystem) ReleaseDir(header *Context, h *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.ReleaseDir(header, h)
}

func (fs *lockingRawFileSystem) Read(header *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	defer fs.locked()()
	return fs.RawFS.Read(header, input, buf)
}

func (fs *lockingRawFileSystem) Write(header *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.RawFS.Write(header, input, data)
}

func (fs *lockingRawFileSystem) Flush(header *Context, input *raw.FlushIn) Status {
	defer fs.locked()()
	return fs.RawFS.Flush(header, input)
}

func (fs *lockingRawFileSystem) Fsync(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fsync(header, input)
}

func (fs *lockingRawFileSystem) ReadDir(out *DirEntryList, header *Context, input *raw.ReadIn) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDir(out, header, input)
}

func (fs *lockingRawFileSystem) FsyncDir(header *Context, input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.FsyncDir(header, input)
}

func (fs *lockingRawFileSystem) Init(params *RawFsInit) {
	defer fs.locked()()
	fs.RawFS.Init(params)
}

func (fs *lockingRawFileSystem) StatFs(out *raw.StatfsOut, context *Context) (code Status) {
	defer fs.locked()()
	return fs.RawFS.StatFs(out, context)
}

func (fs *lockingRawFileSystem) Fallocate(c *Context, in *raw.FallocateIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fallocate(c, in)
}

func (fs *lockingRawFileSystem) String() string {
	defer fs.locked()()
	return fmt.Sprintf("Locked(%s)", fs.RawFS.String())
}
