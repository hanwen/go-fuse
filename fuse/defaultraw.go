package fuse

func (me *DefaultRawFileSystem) Init(init *RawFsInit) {
}

func (me *DefaultRawFileSystem) StatFs(h *InHeader) *StatfsOut {
	return nil
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

func (me *DefaultRawFileSystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return 0, 0, ENOSYS
}

func (me *DefaultRawFileSystem) Read(header *InHeader, input *ReadIn, bp BufferPool) ([]byte, Status) {
	return nil, ENOSYS
}

func (me *DefaultRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
}

func (me *DefaultRawFileSystem) Write(header *InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	return 0, ENOSYS
}

func (me *DefaultRawFileSystem) Flush(header *InHeader, input *FlushIn) Status {
	return OK
}

func (me *DefaultRawFileSystem) Fsync(header *InHeader, input *FsyncIn) (code Status) {
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

func (me *DefaultRawFileSystem) Ioctl(header *InHeader, input *IoctlIn) (output *IoctlOut, data []byte, code Status) {
	return nil, nil, ENOSYS
}
