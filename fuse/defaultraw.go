package fuse

import (
	"github.com/hanwen/go-fuse/raw"
)

var _ = RawFileSystem((*DefaultRawFileSystem)(nil))

func (fs *DefaultRawFileSystem) Init(init *RawFsInit) {
}

func (fs *DefaultRawFileSystem) StatFs(out *StatfsOut, h *raw.InHeader) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Lookup(out *raw.EntryOut, h *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (fs *DefaultRawFileSystem) GetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.GetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Open(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status) {
	return OK
}

func (fs *DefaultRawFileSystem) SetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.SetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Readlink(header *raw.InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) Mknod(out *raw.EntryOut, header *raw.InHeader, input *raw.MknodIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Mkdir(out *raw.EntryOut, header *raw.InHeader, input *raw.MkdirIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Unlink(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Rmdir(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Symlink(out *raw.EntryOut, header *raw.InHeader, pointedTo string, linkName string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Rename(header *raw.InHeader, input *raw.RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Link(out *raw.EntryOut, header *raw.InHeader, input *raw.LinkIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) GetXAttrSize(header *raw.InHeader, attr string) (size int, code Status) {
	return 0, ENOSYS
}

func (fs *DefaultRawFileSystem) GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) SetXAttr(header *raw.InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) RemoveXAttr(header *raw.InHeader, attr string) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Access(header *raw.InHeader, input *raw.AccessIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Create(out *raw.CreateOut, header *raw.InHeader, input *raw.CreateIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) OpenDir(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Read(header *raw.InHeader, input *ReadIn, buf []byte) ReadResult {
	return ReadResult{}
}

func (fs *DefaultRawFileSystem) Release(header *raw.InHeader, input *raw.ReleaseIn) {
}

func (fs *DefaultRawFileSystem) Write(header *raw.InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (fs *DefaultRawFileSystem) Flush(header *raw.InHeader, input *raw.FlushIn) Status {
	return OK
}

func (fs *DefaultRawFileSystem) Fsync(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ReadDir(l *DirEntryList, header *raw.InHeader, input *ReadIn) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ReleaseDir(header *raw.InHeader, input *raw.ReleaseIn) {
}

func (fs *DefaultRawFileSystem) FsyncDir(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}
