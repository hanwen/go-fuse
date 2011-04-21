package fuse

import (
	"log"
)

var _ = log.Println

func (me *DefaultRawFuseFileSystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	return new(InitOut), OK
}

func (me *DefaultRawFuseFileSystem) Destroy(h *InHeader, input *InitIn) {

}

func (me *DefaultRawFuseFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Forget(h *InHeader, input *ForgetIn) {
}

func (me *DefaultRawFuseFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, OK
}

func (me *DefaultRawFuseFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return new(EntryOut), ENOSYS
}

func (me *DefaultRawFuseFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Unlink(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	return 0, 0, nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Read(*ReadIn, *BufferPool) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Release(header *InHeader, input *ReleaseIn) {
}

func (me *DefaultRawFuseFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (me *DefaultRawFuseFileSystem) Flush(input *FlushIn) Status {
	return OK
}

func (me *DefaultRawFuseFileSystem) Fsync(input *FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFuseFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFuseFileSystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
}

func (me *DefaultRawFuseFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	return ENOSYS
}


////////////////////////////////////////////////////////////////
//  DefaultFuseFile

func (me *DefaultFuseFile) Read(*ReadIn, *BufferPool) ([]byte, Status) {
	return []byte(""), ENOSYS
}

func (me *DefaultFuseFile) Write(*WriteIn, []byte) (uint32, Status) {
	return 0, ENOSYS
}

func (me *DefaultFuseFile) Flush() Status {
	return ENOSYS
}

func (me *DefaultFuseFile) Release() {

}

func (me *DefaultFuseFile) Fsync(*FsyncIn) (code Status) {
	return ENOSYS
}


////////////////////////////////////////////////////////////////
// DefaultPathFilesystem

func (me *DefaultPathFilesystem) GetAttr(name string) (*Attr, Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) GetXAttr(name string, attr string) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	return ENOSYS
}

func (me *DefaultPathFilesystem) ListXAttr(name string) ([]string, Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) RemoveXAttr(name string, attr string) Status {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Readlink(name string) (string, Status) {
	return "", ENOSYS
}

func (me *DefaultPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Mkdir(name string, mode uint32) Status {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Unlink(name string) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Rmdir(name string) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Symlink(value string, linkName string) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Rename(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Link(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Open(name string, flags uint32) (file FuseFile, code Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	return OK
}

func (me *DefaultPathFilesystem) Unmount() {
}

func (me *DefaultPathFilesystem) Access(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultPathFilesystem) Create(name string, flags uint32, mode uint32) (file FuseFile, code Status) {
	return nil, ENOSYS
}

func (me *DefaultPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return ENOSYS
}
