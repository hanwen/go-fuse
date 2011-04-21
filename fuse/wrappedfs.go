package fuse

type WrappingPathFilesystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	Original PathFilesystem
}

func (me *WrappingPathFilesystem) GetAttr(name string) (*Attr, Status) {
	return me.Original.GetAttr(name)
}

func (me *WrappingPathFilesystem) Readlink(name string) (string, Status) {
	return me.Original.Readlink(name)
}

func (me *WrappingPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	return me.Original.Mknod(name, mode, dev)
}

func (me *WrappingPathFilesystem) Mkdir(name string, mode uint32) Status {
	return me.Original.Mkdir(name, mode)
}

func (me *WrappingPathFilesystem) Unlink(name string) (code Status) {
	return me.Original.Unlink(name)
}

func (me *WrappingPathFilesystem) Rmdir(name string) (code Status) {
	return me.Original.Rmdir(name)
}

func (me *WrappingPathFilesystem) Symlink(value string, linkName string) (code Status) {
	return me.Original.Symlink(value, linkName)
}

func (me *WrappingPathFilesystem) Rename(oldName string, newName string) (code Status) {
	return me.Original.Rename(oldName, newName)
}

func (me *WrappingPathFilesystem) Link(oldName string, newName string) (code Status) {
	return me.Original.Link(oldName, newName)
}

func (me *WrappingPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	return me.Original.Chmod(name, mode)
}

func (me *WrappingPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return me.Original.Chown(name, uid, gid)
}

func (me *WrappingPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	return me.Original.Truncate(name, offset)
}

func (me *WrappingPathFilesystem) Open(name string, flags uint32) (file FuseFile, code Status) {
	return me.Original.Open(name, flags)
}

func (me *WrappingPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return me.Original.OpenDir(name)
}

func (me *WrappingPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	return me.Original.Mount(conn)
}

func (me *WrappingPathFilesystem) Unmount() {
	me.Original.Unmount()
}

func (me *WrappingPathFilesystem) Access(name string, mode uint32) (code Status) {
	return me.Original.Access(name, mode)
}

func (me *WrappingPathFilesystem) Create(name string, flags uint32, mode uint32) (file FuseFile, code Status) {
	return me.Original.Create(name, flags, mode)
}

func (me *WrappingPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return me.Original.Utimens(name, AtimeNs, CtimeNs)
}

func (me *WrappingPathFilesystem) GetXAttr(name string, attr string) ([]byte, Status) {
	return me.Original.GetXAttr(name, attr)
}

func (me *WrappingPathFilesystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	return me.Original.SetXAttr(name, attr, data, flags)
}

func (me *WrappingPathFilesystem) ListXAttr(name string) ([]string, Status) {
	return me.Original.ListXAttr(name)
}

func (me *WrappingPathFilesystem) RemoveXAttr(name string, attr string) Status {
	return me.Original.RemoveXAttr(name, attr)
}

////////////////////////////////////////////////////////////////
// Wrapping raw FS.

type WrappingRawFilesystem struct {
	Original RawFileSystem
}

func (me *WrappingRawFilesystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	return me.Original.Init(h, input)
}

func (me *WrappingRawFilesystem) Destroy(h *InHeader, input *InitIn) {
	me.Original.Destroy(h, input)
}

func (me *WrappingRawFilesystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return me.Original.Lookup(h, name)
}

func (me *WrappingRawFilesystem) Forget(h *InHeader, input *ForgetIn) {
	me.Original.Forget(h, input)
}

func (me *WrappingRawFilesystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return me.Original.GetAttr(header, input)
}

func (me *WrappingRawFilesystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return me.Original.Open(header, input)
}

func (me *WrappingRawFilesystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return me.Original.SetAttr(header, input)
}

func (me *WrappingRawFilesystem) Readlink(header *InHeader) (out []byte, code Status) {
	return me.Original.Readlink(header)
}

func (me *WrappingRawFilesystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return me.Original.Mknod(header, input, name)
}

func (me *WrappingRawFilesystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return me.Original.Mkdir(header, input, name)
}

func (me *WrappingRawFilesystem) Unlink(header *InHeader, name string) (code Status) {
	return me.Original.Unlink(header, name)
}

func (me *WrappingRawFilesystem) Rmdir(header *InHeader, name string) (code Status) {
	return me.Original.Rmdir(header, name)
}

func (me *WrappingRawFilesystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *WrappingRawFilesystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *WrappingRawFilesystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return me.Original.Link(header, input, name)
}

func (me *WrappingRawFilesystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *WrappingRawFilesystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	return me.Original.GetXAttr(header, attr)
}

func (me *WrappingRawFilesystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	return me.Original.ListXAttr(header)
}

func (me *WrappingRawFilesystem) RemoveXAttr(header *InHeader, attr string) Status {
	return me.Original.RemoveXAttr(header, attr)
}

func (me *WrappingRawFilesystem) Access(header *InHeader, input *AccessIn) (code Status) {
	return me.Original.Access(header, input)
}

func (me *WrappingRawFilesystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	return me.Original.Create(header, input, name)
}

func (me *WrappingRawFilesystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return me.Original.Bmap(header, input)
}

func (me *WrappingRawFilesystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return me.Original.Ioctl(header, input)
}

func (me *WrappingRawFilesystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return me.Original.Poll(header, input)
}

func (me *WrappingRawFilesystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return me.Original.OpenDir(header, input)
}

func (me *WrappingRawFilesystem) Release(header *InHeader, input *ReleaseIn) {
	me.Original.Release(header, input)
}

func (me *WrappingRawFilesystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	return me.Original.Read(input, bp)
}

func (me *WrappingRawFilesystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	return me.Original.Write(input, data)
}

func (me *WrappingRawFilesystem) Flush(input *FlushIn) Status {
	return me.Original.Flush(input)
}

func (me *WrappingRawFilesystem) Fsync(input *FsyncIn) (code Status) {
	return me.Original.Fsync(input)
}

func (me *WrappingRawFilesystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	return me.Original.ReadDir(header, input)
}

func (me *WrappingRawFilesystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
	me.Original.ReleaseDir(header, input)
}

func (me *WrappingRawFilesystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	return me.Original.FsyncDir(header, input)
}
