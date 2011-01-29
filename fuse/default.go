package fuse

func (self *DefaultRawFuseFileSystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	return new(InitOut), OK
}

func (self *DefaultRawFuseFileSystem) Destroy(h *InHeader, input *InitIn) {

}

func (self *DefaultRawFuseFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Forget(h *InHeader, input *ForgetIn) {
}

func (self *DefaultRawFuseFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseFile, status Status) {
	return 0, nil, OK
}

func (self *DefaultRawFuseFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return new(EntryOut), ENOSYS
}

func (self *DefaultRawFuseFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Unlink(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (self *DefaultRawFuseFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (self *DefaultRawFuseFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (self *DefaultRawFuseFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn) Status {
	return ENOSYS
}

func (self *DefaultRawFuseFileSystem) GetXAttr(header *InHeader, input *GetXAttrIn) (out *GetXAttrOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	return ENOSYS
}

func (self *DefaultRawFuseFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, fuseFile RawFuseFile, out *EntryOut, code Status) {
	return 0, nil, nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseDir, status Status) {
	return 0, nil, ENOSYS
}

func (self *DefaultRawFuseFileSystem) Release(header *InHeader, f RawFuseFile) {
}

func (self *DefaultRawFuseFileSystem) ReleaseDir(header *InHeader, f RawFuseDir) {
}


////////////////////////////////////////////////////////////////
//  DefaultRawFuseFile

func (self *DefaultRawFuseFile) Read(*ReadIn, *BufferPool) ([]byte, Status) {
	return []byte(""), ENOSYS
}

func (self *DefaultRawFuseFile) Write(*WriteIn, []byte) (uint32, Status) {
	return 0, ENOSYS
}

func (self *DefaultRawFuseFile) Flush() Status {
	return ENOSYS
}

func (self *DefaultRawFuseFile) Release() {

}

func (self *DefaultRawFuseFile) Fsync(*FsyncIn) (code Status) {
	return ENOSYS
}


////////////////////////////////////////////////////////////////
//

func (self *DefaultRawFuseDir) ReadDir(input *ReadIn) (*DirEntryList, Status) {
	return nil, ENOSYS
}

func (self *DefaultRawFuseDir) ReleaseDir() {
}

func (self *DefaultRawFuseDir) FsyncDir(input *FsyncIn) (code Status) {
	return ENOSYS
}

////////////////////////////////////////////////////////////////
// DefaultPathFilesystem

func (self *DefaultPathFilesystem) GetAttr(name string) (*Attr, Status) {
	return nil, ENOSYS
}

func (self *DefaultPathFilesystem) Readlink(name string) (string, Status) {
	return "", ENOSYS
}

func (self *DefaultPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Mkdir(name string, mode uint32) Status {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Unlink(name string) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Rmdir(name string) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Symlink(value string, linkName string) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Rename(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Link(oldName string, newName string) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Open(name string, flags uint32) (file RawFuseFile, code Status) {
	return nil, ENOSYS
}

func (self *DefaultPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return nil, ENOSYS
}

func (self *DefaultPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	return OK
}

func (self *DefaultPathFilesystem) Unmount() {
}

func (self *DefaultPathFilesystem) Access(name string, mode uint32) (code Status) {
	return ENOSYS
}

func (self *DefaultPathFilesystem) Create(name string, flags uint32, mode uint32) (file RawFuseFile, code Status) {
	return nil, ENOSYS
}

func (self *DefaultPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return ENOSYS
}
