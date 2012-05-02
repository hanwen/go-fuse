package fuse

import (
	"io/ioutil"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

type MutableDataFile struct {
	DefaultFile

	data []byte
	Attr
	GetAttrCalled bool
}

func (f *MutableDataFile) String() string {
	return "MutableDataFile"
}

func (f *MutableDataFile) Read(r *ReadIn, bp BufferPool) ([]byte, Status) {
	return f.data[r.Offset : r.Offset+uint64(r.Size)], OK
}

func (f *MutableDataFile) Write(w *WriteIn, d []byte) (uint32, Status) {
	end := uint64(w.Size) + w.Offset
	if int(end) > len(f.data) {
		data := make([]byte, len(f.data), end)
		copy(data, f.data)
		f.data = data
	}
	copy(f.data[w.Offset:end], d)
	return w.Size, OK
}

func (f *MutableDataFile) Flush() Status {
	return OK
}

func (f *MutableDataFile) Release() {

}

func (f *MutableDataFile) getAttr() *Attr {
	a := f.Attr
	a.Size = uint64(len(f.data))
	return &a
}

func (f *MutableDataFile) GetAttr() (*Attr, Status) {
	f.GetAttrCalled = true
	return f.getAttr(), OK
}

func (f *MutableDataFile) Utimens(atimeNs int64, mtimeNs int64) Status {
	f.Attr.SetNs(atimeNs, mtimeNs, -1)
	return OK
}

func (f *MutableDataFile) Truncate(size uint64) Status {
	f.data = f.data[:size]
	return OK
}

func (f *MutableDataFile) Chown(uid uint32, gid uint32) Status {
	f.Attr.Uid = uid
	f.Attr.Gid = gid
	return OK
}

func (f *MutableDataFile) Chmod(perms uint32) Status {
	f.Attr.Mode = (f.Attr.Mode &^ 07777) | perms
	return OK
}

////////////////

type FSetAttrFs struct {
	DefaultFileSystem
	file *MutableDataFile
}

func (fs *FSetAttrFs) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return nil, ENODATA
}

func (fs *FSetAttrFs) GetAttr(name string, context *Context) (*Attr, Status) {
	if name == "" {
		return &Attr{Mode: S_IFDIR | 0700}, OK
	}
	if name == "file" && fs.file != nil {
		a := fs.file.getAttr()
		a.Mode |= S_IFREG
		return a, OK
	}
	return nil, ENOENT
}

func (fs *FSetAttrFs) Open(name string, flags uint32, context *Context) (File, Status) {
	if name == "file" {
		return fs.file, OK
	}
	return nil, ENOENT
}

func (fs *FSetAttrFs) Create(name string, flags uint32, mode uint32, context *Context) (File, Status) {
	if name == "file" {
		f := NewFile()
		fs.file = f
		fs.file.Attr.Mode = mode
		return f, OK
	}
	return nil, ENOENT
}

func NewFile() *MutableDataFile {
	return &MutableDataFile{}
}

func setupFAttrTest(t *testing.T, fs FileSystem) (dir string, clean func()) {
	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	nfs := NewPathNodeFs(fs, nil)
	state, _, err := MountNodeFileSystem(dir, nfs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()

	go state.Loop()

	// Trigger INIT.
	os.Lstat(dir)
	if state.KernelSettings().Flags&raw.CAP_FILE_OPS == 0 {
		t.Log("Mount does not support file operations")
	}

	return dir, func() {
		if state.Unmount() == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestFSetAttr(t *testing.T) {
	fs := &FSetAttrFs{}
	dir, clean := setupFAttrTest(t, fs)
	defer clean()

	fn := dir + "/file"
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY, 0755)

	CheckSuccess(err)
	defer f.Close()
	fi, err := f.Stat()
	CheckSuccess(err)

	_, err = f.WriteString("hello")
	CheckSuccess(err)

	code := syscall.Ftruncate(int(f.Fd()), 3)
	if code != nil {
		t.Error("truncate retval", os.NewSyscallError("Ftruncate", code))
	}
	if len(fs.file.data) != 3 {
		t.Error("truncate")
	}

	err = f.Chmod(024)
	CheckSuccess(err)
	if fs.file.Attr.Mode&07777 != 024 {
		t.Error("chmod")
	}

	err = os.Chtimes(fn, time.Unix(0, 100e3), time.Unix(0, 101e3))
	CheckSuccess(err)
	if fs.file.Attr.Atimensec != 100e3 || fs.file.Attr.Mtimensec != 101e3 {
		t.Errorf("Utimens: atime %d != 100e3 mtime %d != 101e3",
			fs.file.Attr.Atimensec, fs.file.Attr.Mtimensec)
	}

	newFi, err := f.Stat()
	CheckSuccess(err)
	i1 := ToStatT(fi).Ino
	i2 := ToStatT(newFi).Ino
	if i1 != i2 {
		t.Errorf("f.Lstat().Ino = %d. Returned %d before.", i2, i1)
	}
	// TODO - test chown if run as root.
}
