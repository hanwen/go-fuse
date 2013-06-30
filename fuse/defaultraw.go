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

func (fs *defaultRawFileSystem) StatFs(out *raw.StatfsOut, context *Context) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Lookup(out *raw.EntryOut, context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (fs *defaultRawFileSystem) GetAttr(out *raw.AttrOut, context *Context, input *raw.GetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Open(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status) {
	return OK
}

func (fs *defaultRawFileSystem) SetAttr(out *raw.AttrOut, context *Context, input *raw.SetAttrIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Readlink(context *Context) (out []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) Mknod(out *raw.EntryOut, context *Context, input *raw.MknodIn, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Mkdir(out *raw.EntryOut, context *Context, input *raw.MkdirIn, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Unlink(context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rmdir(context *Context, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Symlink(out *raw.EntryOut, context *Context, pointedTo string, linkName string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rename(context *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Link(out *raw.EntryOut, context *Context, input *raw.LinkIn, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) GetXAttrSize(context *Context, attr string) (size int, code Status) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) GetXAttrData(context *Context, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) SetXAttr(context *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ListXAttr(context *Context) (data []byte, code Status) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) RemoveXAttr(context *Context, attr string) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Access(context *Context, input *raw.AccessIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Create(out *raw.CreateOut, context *Context, input *raw.CreateIn, name string) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) OpenDir(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Read(context *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	return &ReadResultData{}, ENOSYS
}

func (fs *defaultRawFileSystem) Release(context *Context, input *raw.ReleaseIn) {
}

func (fs *defaultRawFileSystem) Write(context *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) Flush(context *Context, input *raw.FlushIn) Status {
	return OK
}

func (fs *defaultRawFileSystem) Fsync(context *Context, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDir(l *DirEntryList, context *Context, input *raw.ReadIn) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDirPlus(l *DirEntryList, context *Context, input *raw.ReadIn) Status {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReleaseDir(context *Context, input *raw.ReleaseIn) {
}

func (fs *defaultRawFileSystem) FsyncDir(context *Context, input *raw.FsyncIn) (code Status) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Fallocate(context *Context, in *raw.FallocateIn) (code Status) {
	return ENOSYS
}
