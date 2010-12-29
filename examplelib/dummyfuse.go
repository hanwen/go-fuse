package examplelib
import (
	"github.com/hanwen/go-fuse/fuse"
	)
// Declare dummy methods, for cut & paste convenience.
type DummyFuse struct{}

func (fs *DummyFuse) Init(h *fuse.InHeader, input *fuse.InitIn) (*fuse.InitOut, fuse.Status) {
	return new(fuse.InitOut), fuse.OK
}

func (fs *DummyFuse) Destroy(h *fuse.InHeader, input *fuse.InitIn) {

}

func (fs *DummyFuse) Lookup(h *fuse.InHeader, name string) (out *fuse.EntryOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DummyFuse) Forget(h *fuse.InHeader, input *fuse.ForgetIn) {
}

func (fs *DummyFuse) GetAttr(header *fuse.InHeader, input *fuse.GetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (fs *DummyFuse) Open(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseFile, status fuse.Status) {
	return 0, nil, fuse.OK
}

func (self *DummyFuse) SetAttr(header *fuse.InHeader, input *fuse.SetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Readlink(header *fuse.InHeader) (out []byte, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Mknod(header *fuse.InHeader, input *fuse.MknodIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	return new(fuse.EntryOut), fuse.ENOSYS
}

func (self *DummyFuse) Mkdir(header *fuse.InHeader, input *fuse.MkdirIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Unlink(header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *DummyFuse) Rmdir(header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *DummyFuse) Symlink(header *fuse.InHeader, pointedTo string, linkName string) (out *fuse.EntryOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Rename(header *fuse.InHeader, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *DummyFuse) Link(header *fuse.InHeader, input *fuse.LinkIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) SetXAttr(header *fuse.InHeader, input *fuse.SetXAttrIn) fuse.Status {
	return fuse.ENOSYS
}

func (self *DummyFuse) GetXAttr(header *fuse.InHeader, input *fuse.GetXAttrIn) (out *fuse.GetXAttrOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Access(header *fuse.InHeader, input *fuse.AccessIn) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *DummyFuse) Create(header *fuse.InHeader, input *fuse.CreateIn, name string) (flags uint32, fuseFile fuse.RawFuseFile, out *fuse.EntryOut, code fuse.Status) {
	return 0, nil, nil, fuse.ENOSYS
}

func (self *DummyFuse) Bmap(header *fuse.InHeader, input *fuse.BmapIn) (out *fuse.BmapOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Ioctl(header *fuse.InHeader, input *fuse.IoctlIn) (out *fuse.IoctlOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) Poll(header *fuse.InHeader, input *fuse.PollIn) (out *fuse.PollOut, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuse) OpenDir(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseDir, status fuse.Status) {
	return 0, nil, fuse.ENOSYS
}

////////////////////////////////////////////////////////////////
//  DummyFuseFile

type DummyFuseFile struct{}

func (self *DummyFuseFile) Read(*fuse.ReadIn) ([]byte, fuse.Status) {
	return []byte(""), fuse.ENOSYS
}

func (self *DummyFuseFile) Write(*fuse.WriteIn, []byte) (uint32, fuse.Status) {
	return 0, fuse.ENOSYS
}


func (self *DummyFuseFile) Flush() {
}
func (self *DummyFuseFile) Release() {

}
func (self *DummyFuseFile) Fsync(*fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyFuseFile) ReadDir(input *fuse.ReadIn) (*fuse.DirEntryList, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyFuseFile) ReleaseDir() {
}
func (self *DummyFuseFile) FsyncDir(input *fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOSYS
}

////////////////////////////////////////////////////////////////
// DummyPathFuse

type DummyPathFuse struct{}

func (self *DummyPathFuse) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyPathFuse) Readlink(name string) (string, fuse.Status) {
	return "", fuse.ENOSYS
}

func (self *DummyPathFuse) Mknod(name string, mode uint32, dev uint32) fuse.Status {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Mkdir(name string, mode uint32) fuse.Status {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Unlink(name string) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Rmdir(name string) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Symlink(value string, linkName string) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Rename(oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Link(oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Chmod(name string, mode uint32) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Chown(name string, uid uint32, gid uint32) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Truncate(name string, offset uint64) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Open(name string, flags uint32) (file fuse.RawFuseFile, code fuse.Status) {
	return nil, fuse.ENOSYS
}


func (self *DummyPathFuse) OpenDir(name string) (dir fuse.RawFuseDir, code fuse.Status) {
	return nil, fuse.ENOSYS
}


func (self *DummyPathFuse) Init() (*fuse.InitOut, fuse.Status) {
	return nil, fuse.ENOSYS
}


func (self *DummyPathFuse) Destroy() {
}
func (self *DummyPathFuse) Access(name string, mode uint32) (code fuse.Status) {
	return fuse.ENOSYS
}
func (self *DummyPathFuse) Create(name string, flags uint32, mode uint32) (file fuse.RawFuseFile, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *DummyPathFuse) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *DummyPathFuse) SetOptions(*fuse.PathFileSystemConnectorOptions) {
	
}
