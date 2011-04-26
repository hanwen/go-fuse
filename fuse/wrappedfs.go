package fuse

type WrappingFileSystem struct {
	// Should be public so people reusing can access the wrapped
	// FS.
	Original FileSystem
}

func (me *WrappingFileSystem) GetAttr(name string) (*Attr, Status) {
	return me.Original.GetAttr(name)
}

func (me *WrappingFileSystem) Readlink(name string) (string, Status) {
	return me.Original.Readlink(name)
}

func (me *WrappingFileSystem) Mknod(name string, mode uint32, dev uint32) Status {
	return me.Original.Mknod(name, mode, dev)
}

func (me *WrappingFileSystem) Mkdir(name string, mode uint32) Status {
	return me.Original.Mkdir(name, mode)
}

func (me *WrappingFileSystem) Unlink(name string) (code Status) {
	return me.Original.Unlink(name)
}

func (me *WrappingFileSystem) Rmdir(name string) (code Status) {
	return me.Original.Rmdir(name)
}

func (me *WrappingFileSystem) Symlink(value string, linkName string) (code Status) {
	return me.Original.Symlink(value, linkName)
}

func (me *WrappingFileSystem) Rename(oldName string, newName string) (code Status) {
	return me.Original.Rename(oldName, newName)
}

func (me *WrappingFileSystem) Link(oldName string, newName string) (code Status) {
	return me.Original.Link(oldName, newName)
}

func (me *WrappingFileSystem) Chmod(name string, mode uint32) (code Status) {
	return me.Original.Chmod(name, mode)
}

func (me *WrappingFileSystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return me.Original.Chown(name, uid, gid)
}

func (me *WrappingFileSystem) Truncate(name string, offset uint64) (code Status) {
	return me.Original.Truncate(name, offset)
}

func (me *WrappingFileSystem) Open(name string, flags uint32) (file File, code Status) {
	return me.Original.Open(name, flags)
}

func (me *WrappingFileSystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return me.Original.OpenDir(name)
}

func (me *WrappingFileSystem) Mount(conn *FileSystemConnector) Status {
	return me.Original.Mount(conn)
}

func (me *WrappingFileSystem) Unmount() {
	me.Original.Unmount()
}

func (me *WrappingFileSystem) Access(name string, mode uint32) (code Status) {
	return me.Original.Access(name, mode)
}

func (me *WrappingFileSystem) Create(name string, flags uint32, mode uint32) (file File, code Status) {
	return me.Original.Create(name, flags, mode)
}

func (me *WrappingFileSystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return me.Original.Utimens(name, AtimeNs, CtimeNs)
}

func (me *WrappingFileSystem) GetXAttr(name string, attr string) ([]byte, Status) {
	return me.Original.GetXAttr(name, attr)
}

func (me *WrappingFileSystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	return me.Original.SetXAttr(name, attr, data, flags)
}

func (me *WrappingFileSystem) ListXAttr(name string) ([]string, Status) {
	return me.Original.ListXAttr(name)
}

func (me *WrappingFileSystem) RemoveXAttr(name string, attr string) Status {
	return me.Original.RemoveXAttr(name, attr)
}

////////////////////////////////////////////////////////////////
// Wrapping raw FS.

type WrappingRawFileSystem struct {
	Original RawFileSystem
}

func (me *WrappingRawFileSystem) Destroy(h *InHeader, input *InitIn) {
	me.Original.Destroy(h, input)
}

func (me *WrappingRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return me.Original.Lookup(h, name)
}

func (me *WrappingRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
	me.Original.Forget(h, input)
}

func (me *WrappingRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return me.Original.GetAttr(header, input)
}

func (me *WrappingRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return me.Original.Open(header, input)
}

func (me *WrappingRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return me.Original.SetAttr(header, input)
}

func (me *WrappingRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	return me.Original.Readlink(header)
}

func (me *WrappingRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return me.Original.Mknod(header, input, name)
}

func (me *WrappingRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return me.Original.Mkdir(header, input, name)
}

func (me *WrappingRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	return me.Original.Unlink(header, name)
}

func (me *WrappingRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	return me.Original.Rmdir(header, name)
}

func (me *WrappingRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *WrappingRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *WrappingRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return me.Original.Link(header, input, name)
}

func (me *WrappingRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *WrappingRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	return me.Original.GetXAttr(header, attr)
}

func (me *WrappingRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	return me.Original.ListXAttr(header)
}

func (me *WrappingRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	return me.Original.RemoveXAttr(header, attr)
}

func (me *WrappingRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	return me.Original.Access(header, input)
}

func (me *WrappingRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	return me.Original.Create(header, input, name)
}

func (me *WrappingRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	return me.Original.OpenDir(header, input)
}

func (me *WrappingRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
	me.Original.Release(header, input)
}

func (me *WrappingRawFileSystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	return me.Original.Read(input, bp)
}

func (me *WrappingRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	return me.Original.Write(input, data)
}

func (me *WrappingRawFileSystem) Flush(input *FlushIn) Status {
	return me.Original.Flush(input)
}

func (me *WrappingRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	return me.Original.Fsync(input)
}

func (me *WrappingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	return me.Original.ReadDir(header, input)
}

func (me *WrappingRawFileSystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
	me.Original.ReleaseDir(header, input)
}

func (me *WrappingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	return me.Original.FsyncDir(header, input)
}
