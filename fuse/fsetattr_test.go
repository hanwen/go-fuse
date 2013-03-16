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

func (f *MutableDataFile) Read(buf []byte, off int64) (ReadResult, Status) {
	end := int(off) + len(buf)
	if end > len(f.data) {
		end = len(f.data)
	}

	return &ReadResultData{Data: f.data[off:end]}, OK
}

func (f *MutableDataFile) Write(d []byte, off int64) (uint32, Status) {
	end := int64(len(d)) + off
	if int(end) > len(f.data) {
		data := make([]byte, len(f.data), end)
		copy(data, f.data)
		f.data = data[:end]
	}
	copy(f.data[off:end], d)
	f.Attr.Size = uint64(len(f.data))

	return uint32(end - off), OK
}

func (f *MutableDataFile) Flush() Status {
	return OK
}

func (f *MutableDataFile) Release() {

}

func (f *MutableDataFile) getAttr(out *Attr) {
	*out = f.Attr
	out.Size = uint64(len(f.data))
}

func (f *MutableDataFile) GetAttr(out *Attr) Status {
	f.GetAttrCalled = true
	f.getAttr(out)
	return OK
}

func (f *MutableDataFile) Utimens(atime *time.Time, mtime *time.Time) Status {
	f.Attr.SetTimes(atime, mtime, nil)
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

// This FS only supports a single r/w file called "/file".
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
		var a Attr
		fs.file.getAttr(&a)
		a.Mode |= S_IFREG
		return &a, OK
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

func setupFAttrTest(t *testing.T, fs FileSystem) (dir string, clean func(), sync func()) {
	dir, err := ioutil.TempDir("", "go-fuse-fsetattr_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	nfs := NewPathNodeFs(fs, nil)
	state, _, err := MountNodeFileSystem(dir, nfs, nil)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
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
		}, func() {
			state.ThreadSanitizerSync()
		}
}

func TestDataReadLarge(t *testing.T) {
	fs := &FSetAttrFs{}
	dir, clean, _ := setupFAttrTest(t, fs)
	defer clean()

	content := RandomData(385 * 1023)
	fn := dir + "/file"
	err := ioutil.WriteFile(fn, []byte(content), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	back, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	CompareSlices(t, back, content)
}

func TestFSetAttr(t *testing.T) {
	fs := &FSetAttrFs{}
	dir, clean, sync := setupFAttrTest(t, fs)
	defer clean()

	fn := dir + "/file"
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY, 0755)

	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	_, err = f.WriteString("hello")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	code := syscall.Ftruncate(int(f.Fd()), 3)
	if code != nil {
		t.Error("truncate retval", os.NewSyscallError("Ftruncate", code))
	}

	sync()
	if len(fs.file.data) != 3 {
		t.Error("truncate")
	}

	err = f.Chmod(024)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	sync()
	if fs.file.Attr.Mode&07777 != 024 {
		t.Error("chmod")
	}

	err = os.Chtimes(fn, time.Unix(0, 100e3), time.Unix(0, 101e3))
	if err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	sync()
	if fs.file.Attr.Atimensec != 100e3 || fs.file.Attr.Mtimensec != 101e3 {
		t.Errorf("Utimens: atime %d != 100e3 mtime %d != 101e3",
			fs.file.Attr.Atimensec, fs.file.Attr.Mtimensec)
	}

	newFi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	i1 := ToStatT(fi).Ino
	i2 := ToStatT(newFi).Ino
	if i1 != i2 {
		t.Errorf("f.Lstat().Ino = %d. Returned %d before.", i2, i1)
	}
	// TODO - test chown if run as root.
}
