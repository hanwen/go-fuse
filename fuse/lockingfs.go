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

func (fs *lockingRawFileSystem) Lookup(header *raw.InHeader, name string, out *raw.EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Lookup(header, name, out)
}

func (fs *lockingRawFileSystem) SetDebug(dbg bool) {
	defer fs.locked()()
	fs.RawFS.SetDebug(dbg)
}

func (fs *lockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.RawFS.Forget(nodeID, nlookup)
}

func (fs *lockingRawFileSystem) GetAttr(input *raw.GetAttrIn, out *raw.AttrOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.GetAttr(input, out)
}

func (fs *lockingRawFileSystem) Open(input *raw.OpenIn, out *raw.OpenOut) (status Status) {

	defer fs.locked()()
	return fs.RawFS.Open(input, out)
}

func (fs *lockingRawFileSystem) SetAttr(input *raw.SetAttrIn, out *raw.AttrOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetAttr(input, out)
}

func (fs *lockingRawFileSystem) Readlink(header *raw.InHeader) (out []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.Readlink(header)
}

func (fs *lockingRawFileSystem) Mknod(input *raw.MknodIn, name string, out *raw.EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mknod(input, name, out)
}

func (fs *lockingRawFileSystem) Mkdir(input *raw.MkdirIn, name string, out *raw.EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mkdir(input, name, out)
}

func (fs *lockingRawFileSystem) Unlink(header *raw.InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Unlink(header, name)
}

func (fs *lockingRawFileSystem) Rmdir(header *raw.InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rmdir(header, name)
}

func (fs *lockingRawFileSystem) Symlink(header *raw.InHeader, pointedTo string, linkName string, out *raw.EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Symlink(header, pointedTo, linkName, out)
}

func (fs *lockingRawFileSystem) Rename(input *raw.RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rename(input, oldName, newName)
}

func (fs *lockingRawFileSystem) Link(input *raw.LinkIn, name string, out *raw.EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Link(input, name, out)
}

func (fs *lockingRawFileSystem) SetXAttr(input *raw.SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.RawFS.SetXAttr(input, attr, data)
}

func (fs *lockingRawFileSystem) GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrData(header, attr)
}

func (fs *lockingRawFileSystem) GetXAttrSize(header *raw.InHeader, attr string) (sz int, code Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttrSize(header, attr)
}

func (fs *lockingRawFileSystem) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.ListXAttr(header)
}

func (fs *lockingRawFileSystem) RemoveXAttr(header *raw.InHeader, attr string) Status {
	defer fs.locked()()
	return fs.RawFS.RemoveXAttr(header, attr)
}

func (fs *lockingRawFileSystem) Access(input *raw.AccessIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Access(input)
}

func (fs *lockingRawFileSystem) Create(input *raw.CreateIn, name string, out *raw.CreateOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Create(input, name, out)
}

func (fs *lockingRawFileSystem) OpenDir(input *raw.OpenIn, out *raw.OpenOut) (status Status) {
	defer fs.locked()()
	return fs.RawFS.OpenDir(input, out)
}

func (fs *lockingRawFileSystem) Release(input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.Release(input)
}

func (fs *lockingRawFileSystem) ReleaseDir(input *raw.ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.ReleaseDir(input)
}

func (fs *lockingRawFileSystem) Read(input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	defer fs.locked()()
	return fs.RawFS.Read(input, buf)
}

func (fs *lockingRawFileSystem) Write(input *raw.WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.RawFS.Write(input, data)
}

func (fs *lockingRawFileSystem) Flush(input *raw.FlushIn) Status {
	defer fs.locked()()
	return fs.RawFS.Flush(input)
}

func (fs *lockingRawFileSystem) Fsync(input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fsync(input)
}

func (fs *lockingRawFileSystem) ReadDir(input *raw.ReadIn, out *DirEntryList) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDir(input, out)
}

func (fs *lockingRawFileSystem) ReadDirPlus(input *raw.ReadIn, out *DirEntryList) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDirPlus(input, out)
}

func (fs *lockingRawFileSystem) FsyncDir(input *raw.FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.FsyncDir(input)
}

func (fs *lockingRawFileSystem) Init(s *Server) {
	defer fs.locked()()
	fs.RawFS.Init(s)
}

func (fs *lockingRawFileSystem) StatFs(header *raw.InHeader, out *raw.StatfsOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.StatFs(header, out)
}

func (fs *lockingRawFileSystem) Fallocate(in *raw.FallocateIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fallocate(in)
}

func (fs *lockingRawFileSystem) String() string {
	defer fs.locked()()
	return fmt.Sprintf("Locked(%s)", fs.RawFS.String())
}
