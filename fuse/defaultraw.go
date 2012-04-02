package fuse

import (
	"github.com/hanwen/go-fuse/raw"
)

func (me *DefaultRawFileSystem) Init(init *RawFsInit) {
}

func (me *DefaultRawFileSystem) StatFs(h *raw.InHeader) *StatfsOut {
	return nil
}

func (me *DefaultRawFileSystem) Lookup(h *raw.InHeader, name string) (out *raw.EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (me *DefaultRawFileSystem) GetAttr(header *raw.InHeader, input *raw.GetAttrIn) (out *raw.AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Open(header *raw.InHeader, input *raw.OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, OK
}

func (me *DefaultRawFileSystem) SetAttr(header *raw.InHeader, input *raw.SetAttrIn) (out *raw.AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Readlink(header *raw.InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Mknod(header *raw.InHeader, input *raw.MknodIn, name string) (out *raw.EntryOut, code Status) {
	return new(raw.EntryOut), ENOSYS
}

func (me *DefaultRawFileSystem) Mkdir(header *raw.InHeader, input *raw.MkdirIn, name string) (out *raw.EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Unlink(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Rmdir(header *raw.InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Symlink(header *raw.InHeader, pointedTo string, linkName string) (out *raw.EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Rename(header *raw.InHeader, input *raw.RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Link(header *raw.InHeader, input *raw.LinkIn, name string) (out *raw.EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) GetXAttrSize(header *raw.InHeader, attr string) (size int, code Status) {
	return 0, ENOSYS
}

func (me *DefaultRawFileSystem) GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) SetXAttr(header *raw.InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (me *DefaultRawFileSystem) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) RemoveXAttr(header *raw.InHeader, attr string) Status {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Access(header *raw.InHeader, input *raw.AccessIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Create(header *raw.InHeader, input *raw.CreateIn, name string) (flags uint32, handle uint64, out *raw.EntryOut, code Status) {
	return 0, 0, nil, ENOSYS
}

func (me *DefaultRawFileSystem) Bmap(header *raw.InHeader, input *raw.BmapIn) (out *raw.BmapOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Poll(header *raw.InHeader, input *raw.PollIn) (out *raw.PollOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) OpenDir(header *raw.InHeader, input *raw.OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, ENOSYS
}

func (me *DefaultRawFileSystem) Read(header *raw.InHeader, input *ReadIn, bp BufferPool) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Release(header *raw.InHeader, input *raw.ReleaseIn) {
}

func (me *DefaultRawFileSystem) Write(header *raw.InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (me *DefaultRawFileSystem) Flush(header *raw.InHeader, input *raw.FlushIn) Status {
	return OK
}

func (me *DefaultRawFileSystem) Fsync(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) ReadDir(header *raw.InHeader, input *ReadIn) (*DirEntryList, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) ReleaseDir(header *raw.InHeader, input *raw.ReleaseIn) {
}

func (me *DefaultRawFileSystem) FsyncDir(header *raw.InHeader, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Ioctl(header *raw.InHeader, input *raw.IoctlIn) (output *raw.IoctlOut, data []byte, code Status) {
	return nil, nil, ENOSYS
}
