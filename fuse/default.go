package fuse

import (
	"fmt"
	"log"
)

var _ = log.Println
var _ = fmt.Println

func (me *DefaultRawFileSystem) Destroy(h *InHeader, input *InitIn) {

}

func (me *DefaultRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
}

func (me *DefaultRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, OK
}

func (me *DefaultRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return new(EntryOut), ENOSYS
}

func (me *DefaultRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	return ENOSYS
}

func (me *DefaultRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	return 0, 0, nil, ENOSYS
}

func (me *DefaultRawFileSystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, ENOSYS
}

func (me *DefaultRawFileSystem) Read(*ReadIn, *BufferPool) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
}

func (me *DefaultRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (me *DefaultRawFileSystem) Flush(input *FlushIn) Status {
	return OK
}

func (me *DefaultRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
}

func (me *DefaultRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	return ENOSYS
}

////////////////////////////////////////////////////////////////
//  DefaultFile

func (me *DefaultFile) Read(*ReadIn, *BufferPool) ([]byte, Status) {
	return []byte(""), ENOSYS
}

func (me *DefaultFile) Write(*WriteIn, []byte) (uint32, Status) {
	return 0, ENOSYS
}

func (me *DefaultFile) Flush() Status {
	return ENOSYS
}

func (me *DefaultFile) Release() {

}

func (me *DefaultFile) GetAttr() (*Attr, Status) {
	return nil, ENOSYS
}

func (me *DefaultFile) Fsync(*FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultFile) Utimens(atimeNs uint64, mtimeNs uint64) Status {
	return ENOSYS
}

func (me *DefaultFile) Truncate(size uint64) Status {
	return ENOSYS
}

func (me *DefaultFile) Chown(uid uint32, gid uint32) Status {
	return ENOSYS
}

func (me *DefaultFile) Chmod(perms uint32) Status {
	return ENOSYS
}

////////////////////////////////////////////////////////////////
// DefaultFileSystem

func (me *DefaultFileSystem) GetAttr(name string) (*Attr, Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	return ENOSYS
}

func (me *DefaultFileSystem) ListXAttr(name string) ([]string, Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) RemoveXAttr(name string, attr string) Status {
	return ENOSYS
}

func (me *DefaultFileSystem) Readlink(name string) (string, Status) {
	return "", ENOSYS
}

func (me *DefaultFileSystem) Mknod(name string, mode uint32, dev uint32) Status {
	return ENOSYS
}

func (me *DefaultFileSystem) Mkdir(name string, mode uint32) Status {
	return ENOSYS
}

func (me *DefaultFileSystem) Unlink(name string) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Rmdir(name string) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Symlink(value string, linkName string) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Rename(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Link(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Chmod(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Truncate(name string, offset uint64) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Open(name string, flags uint32) (file File, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) Mount(conn *FileSystemConnector) Status {
	return OK
}

func (me *DefaultFileSystem) Unmount() {
}

func (me *DefaultFileSystem) Access(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (me *DefaultFileSystem) Create(name string, flags uint32, mode uint32) (file File, code Status) {
	return nil, ENOSYS
}

func (me *DefaultFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return ENOSYS
}
