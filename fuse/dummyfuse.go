package fuse

// Declare dummy methods, for cut & paste convenience.
type DummyFuse struct{}

func (fs *DummyFuse) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	return new(InitOut), OK
}

func (fs *DummyFuse) Destroy(h *InHeader, input *InitIn) {

}

func (fs *DummyFuse) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (fs *DummyFuse) Forget(h *InHeader, input *ForgetIn) {
}

func (fs *DummyFuse) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (fs *DummyFuse) Open(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseFile, status Status) {
	return 0, nil, OK
}

func (self *DummyFuse) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Readlink(header *InHeader) (out []byte, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	return new(EntryOut), ENOSYS
}

func (self *DummyFuse) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Unlink(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (self *DummyFuse) Rmdir(header *InHeader, name string) (code Status) {
	return ENOSYS
}

func (self *DummyFuse) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	return ENOSYS
}

func (self *DummyFuse) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) SetXAttr(header *InHeader, input *SetXAttrIn) Status {
	return ENOSYS
}

func (self *DummyFuse) GetXAttr(header *InHeader, input *GetXAttrIn) (out *GetXAttrOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Access(header *InHeader, input *AccessIn) (code Status) {
	return ENOSYS
}

func (self *DummyFuse) Create(header *InHeader, input *CreateIn, name string) (flags uint32, fuseFile RawFuseFile, out *EntryOut, code Status) {
	return 0, nil, nil, ENOSYS
}

func (self *DummyFuse) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}

func (self *DummyFuse) OpenDir(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseDir, status Status) {
	return 0, nil, ENOSYS
}

////////////////////////////////////////////////////////////////
//  DummyFuseFile

type DummyFuseFile struct{}

func (self *DummyFuseFile) Read(*ReadIn) ([]byte, Status) {
	return []byte(""), ENOSYS
}

func (self *DummyFuseFile) Write(*WriteIn, []byte) (uint32, Status) {
	return 0, ENOSYS
}


func (self *DummyFuseFile) Flush() {
}
func (self *DummyFuseFile) Release() {

}
func (self *DummyFuseFile) Fsync(*FsyncIn) (code Status) {
	return ENOSYS
}
func (self *DummyFuseFile) ReadDir(input *ReadIn) (*DEntryList, Status) {
	return nil, ENOSYS
}

func (self *DummyFuseFile) ReleaseDir() {
}
func (self *DummyFuseFile) FsyncDir(input *FsyncIn) (code Status) {
	return ENOSYS
}

////////////////////////////////////////////////////////////////
// DummyPathFuse

type DummyPathFuse struct {}

func (self *DummyPathFuse) GetAttr(name string) (*Attr, Status) {
	return nil, ENOSYS
}

func (self *DummyPathFuse) Readlink(name string) (string, Status) {
	return "", ENOSYS
}

func (self *DummyPathFuse) Mknod(name string, mode uint32, dev uint32) Status {
	return ENOSYS
}
func (self *DummyPathFuse) Mkdir(name string, mode uint32) Status {
	return ENOSYS
}
func (self *DummyPathFuse) Unlink(name string) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Rmdir(name string) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Symlink(value string, linkName string) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Rename(oldName string, newName string) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Link(oldName string, newName string) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Chmod(name string, mode uint32) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Chown(name string, uid uint32, gid uint32) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Truncate(name string, offset uint64) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Open(name string, flags uint32) (file RawFuseFile, code Status) {
	return nil, ENOSYS
}


func (self *DummyPathFuse) OpenDir(name string) (dir RawFuseDir, code Status) {
	return nil, ENOSYS
}


func (self *DummyPathFuse) Init() (*InitOut, Status) {
	return nil, ENOSYS
}


func (self *DummyPathFuse) Destroy() {
}
func (self *DummyPathFuse) Access(name string, mode uint32) (code Status) {
	return ENOSYS
}
func (self *DummyPathFuse) Create(name string, flags uint32, mode uint32) (file RawFuseFile, code Status) {
	return nil, ENOSYS
}

func (self *DummyPathFuse) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return ENOSYS
}
