package fuse

import (
	"io/ioutil"
	"os"
	"syscall"
	"testing"
)

type MutableDataFile struct {
	DefaultFile

	data []byte
	os.FileInfo
	GetAttrCalled bool
}

func (me *MutableDataFile) Read(r *ReadIn, bp BufferPool) ([]byte, Status) {
	return me.data[r.Offset : r.Offset+uint64(r.Size)], OK
}

func (me *MutableDataFile) Write(w *WriteIn, d []byte) (uint32, Status) {
	end := uint64(w.Size) + w.Offset
	if int(end) > len(me.data) {
		data := make([]byte, len(me.data), end)
		copy(data, me.data)
		me.data = data
	}
	copy(me.data[w.Offset:end], d)
	return w.Size, OK
}

func (me *MutableDataFile) Flush() Status {
	return OK
}

func (me *MutableDataFile) Release() {

}

func (me *MutableDataFile) getAttr() *os.FileInfo {
	f := me.FileInfo
	f.Size = int64(len(me.data))
	return &f
}

func (me *MutableDataFile) GetAttr() (*os.FileInfo, Status) {
	me.GetAttrCalled = true
	return me.getAttr(), OK
}

func (me *MutableDataFile) Fsync(*FsyncIn) (code Status) {
	return OK
}

func (me *MutableDataFile) Utimens(atimeNs uint64, mtimeNs uint64) Status {
	me.FileInfo.Atime_ns = int64(atimeNs)
	me.FileInfo.Mtime_ns = int64(mtimeNs)
	return OK
}

func (me *MutableDataFile) Truncate(size uint64) Status {
	me.data = me.data[:size]
	return OK
}

func (me *MutableDataFile) Chown(uid uint32, gid uint32) Status {
	me.FileInfo.Uid = int(uid)
	me.FileInfo.Gid = int(uid)
	return OK
}

func (me *MutableDataFile) Chmod(perms uint32) Status {
	me.FileInfo.Mode = (me.FileInfo.Mode &^ 07777) | perms
	return OK
}

////////////////

type FSetAttrFs struct {
	DefaultFileSystem
	file *MutableDataFile
}

func (me *FSetAttrFs) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return nil, ENODATA
}

func (me *FSetAttrFs) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	if name == "" {
		return &os.FileInfo{Mode: S_IFDIR | 0700}, OK
	}
	if name == "file" && me.file != nil {
		a := me.file.getAttr()
		a.Mode |= S_IFREG
		return a, OK
	}
	return nil, ENOENT
}

func (me *FSetAttrFs) Open(name string, flags uint32, context *Context) (File, Status) {
	if name == "file" {
		return me.file, OK
	}
	return nil, ENOENT
}

func (me *FSetAttrFs) Create(name string, flags uint32, mode uint32, context *Context) (File, Status) {
	if name == "file" {
		f := NewFile()
		me.file = f
		me.file.FileInfo.Mode = mode
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
	state, _, err := MountPathFileSystem(dir, fs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()

	go state.Loop()

	// Trigger INIT.
	os.Lstat(dir)
	if state.KernelSettings().Flags&CAP_FILE_OPS == 0 {
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

	code := syscall.Ftruncate(f.Fd(), 3)
	if code != 0 {
		t.Error("truncate retval", os.NewSyscallError("Ftruncate", code))
	}
	if len(fs.file.data) != 3 {
		t.Error("truncate")
	}

	err = f.Chmod(024)
	CheckSuccess(err)
	if fs.file.FileInfo.Mode&07777 != 024 {
		t.Error("chmod")
	}

	err = os.Chtimes(fn, 100e3, 101e3)
	CheckSuccess(err)
	if fs.file.FileInfo.Atime_ns != 100e3 || fs.file.FileInfo.Mtime_ns != 101e3 {
		t.Errorf("Utimens: atime %d != 100e3 mtime %d != 101e3",
			fs.file.FileInfo.Atime_ns, fs.file.FileInfo.Mtime_ns)
	}

	newFi, err := f.Stat()
	CheckSuccess(err)
	if fi.Ino != newFi.Ino {
		t.Errorf("f.Lstat().Ino = %d. Returned %d before.", newFi.Ino, fi.Ino)
	}
	// TODO - test chown if run as root.
}
