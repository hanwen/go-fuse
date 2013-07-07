package fuse

import (
	"github.com/hanwen/go-fuse/raw"

	"os"
)

// NewDefaultRawFileSystem returns ENOSYS (not implemented) for all
// operations.
func NewDefaultRawFileSystem() RawFileSystem {
	return (*defaultRawFileSystem)(nil)
}

type defaultRawFileSystem struct{}

func (fs *defaultRawFileSystem) Init(*Server) {
}

func (fs *defaultRawFileSystem) String() string {
	return os.Args[0]
}

func (fs *defaultRawFileSystem) SetDebug(dbg bool) {
}

func (fs *defaultRawFileSystem) StatFs(header *raw.InHeader, out *raw.StatfsOut) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Lookup(header *raw.InHeader, name string, out *raw.EntryOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (fs *defaultRawFileSystem) GetAttr(input *raw.GetAttrIn, out *raw.AttrOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Open(input *raw.OpenIn, out *raw.OpenOut) (status Status) {
	return OK
}

func (fs *defaultRawFileSystem) SetAttr(input *raw.SetAttrIn, out *raw.AttrOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Readlink(header *raw.InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) Mknod(input *raw.MknodIn, name string, out *raw.EntryOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Mkdir(input *raw.MkdirIn, name string, out *raw.EntryOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Unlink(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rmdir(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Symlink(header *raw.InHeader, pointedTo string, linkName string, out *raw.EntryOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rename(input *raw.RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Link(input *raw.LinkIn, name string, out *raw.EntryOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) GetXAttrSize(header *raw.InHeader, attr string) (size int, code Status) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) SetXAttr(input *raw.SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) RemoveXAttr(header *raw.InHeader, attr string) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Access(input *raw.AccessIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Create(input *raw.CreateIn, name string, out *raw.CreateOut) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) OpenDir(input *raw.OpenIn, out *raw.OpenOut) (status Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Read(input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) Release(input *raw.ReleaseIn) {
}

func (fs *defaultRawFileSystem) Write(input *raw.WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) Flush(input *raw.FlushIn) Status {
	return OK
}

func (fs *defaultRawFileSystem) Fsync(input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDir(input *raw.ReadIn, l *DirEntryList) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDirPlus(input *raw.ReadIn, l *DirEntryList) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReleaseDir(input *raw.ReleaseIn) {
}

func (fs *defaultRawFileSystem) FsyncDir(input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Fallocate(in *raw.FallocateIn) (code Status) {
	return ENOSYS
}
