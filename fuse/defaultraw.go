package fuse

import (
	"github.com/hanwen/go-fuse/raw"

	"os"
)

var _ = RawFileSystem((*DefaultRawFileSystem)(nil))

func (fs *DefaultRawFileSystem) Init(init *RawFsInit) {
}

func (fs *DefaultRawFileSystem) String() string {
	return os.Args[0]
}

func (fs *DefaultRawFileSystem) StatFs(out *StatfsOut, context *Context) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Lookup(out *raw.EntryOut, context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (fs *DefaultRawFileSystem) GetAttr(out *raw.AttrOut, context *Context, input *raw.GetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Open(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status) {
	return OK
}

func (fs *DefaultRawFileSystem) SetAttr(out *raw.AttrOut, context *Context, input *raw.SetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Readlink(context *Context) (out []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) Mknod(out *raw.EntryOut, context *Context, input *raw.MknodIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Mkdir(out *raw.EntryOut, context *Context, input *raw.MkdirIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Unlink(context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Rmdir(context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Symlink(out *raw.EntryOut, context *Context, pointedTo string, linkName string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Rename(context *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Link(out *raw.EntryOut, context *Context, input *raw.LinkIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) GetXAttrSize(context *Context, attr string) (size int, code Status) {
	return 0, ENOSYS
}

func (fs *DefaultRawFileSystem) GetXAttrData(context *Context, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) SetXAttr(context *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ListXAttr(context *Context) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultRawFileSystem) RemoveXAttr(context *Context, attr string) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Access(context *Context, input *raw.AccessIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Create(out *raw.CreateOut, context *Context, input *raw.CreateIn, name string) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) OpenDir(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Read(context *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	return &ReadResultData{}, ENOSYS
}

func (fs *DefaultRawFileSystem) Release(context *Context, input *raw.ReleaseIn) {
}

func (fs *DefaultRawFileSystem) Write(context *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (fs *DefaultRawFileSystem) Flush(context *Context, input *raw.FlushIn) Status {
	return OK
}

func (fs *DefaultRawFileSystem) Fsync(context *Context, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ReadDir(l *DirEntryList, context *Context, input *raw.ReadIn) Status {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) ReleaseDir(context *Context, input *raw.ReleaseIn) {
}

func (fs *DefaultRawFileSystem) FsyncDir(context *Context, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *DefaultRawFileSystem) Fallocate(context *Context, in *raw.FallocateIn) (code Status) {
	return ENOSYS
}
