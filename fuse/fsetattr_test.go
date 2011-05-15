package fuse

import (
	"fmt"
	"log"
	"json"
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

func (me *FSetAttrFs) GetXAttr(name string, attr string) ([]byte, Status) {
	return nil, syscall.ENODATA
}

func (me *FSetAttrFs) GetAttr(name string) (*os.FileInfo, Status) {
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

func (me *FSetAttrFs) Open(name string, flags uint32) (File, Status) {
	if name == "file" {
		return me.file, OK
	}
	return nil, ENOENT
}

func (me *FSetAttrFs) Create(name string, flags uint32, mode uint32) (File, Status) {
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

func TestFSetAttr(t *testing.T) {
	fs := &FSetAttrFs{}

	dir := MakeTempDir()
	defer os.RemoveAll(dir)
	state, _, err := MountFileSystem(dir, fs, nil)
	CheckSuccess(err)
	state.Debug = true
	defer state.Unmount()

	go state.Loop(false)

	fn := dir + "/file"
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY, 0755)
	CheckSuccess(err)
	defer f.Close()

	_, err = f.WriteString("hello")
	CheckSuccess(err)

	fmt.Println("Ftruncate")
	code := syscall.Ftruncate(f.Fd(), 3)
	if code != 0 {
		t.Error("truncate retval", os.NewSyscallError("Ftruncate", code))
	}
	if len(fs.file.data) != 3 {
		t.Error("truncate")
	}

	if state.KernelSettings().Flags&CAP_FILE_OPS == 0 {
		log.Println("Mount does not support file operations")
		m, _ := json.Marshal(state.KernelSettings())
		log.Println("Kernel settings: ", string(m))
		return
	}

	_, err = os.Lstat(fn)
	CheckSuccess(err)

	if !fs.file.GetAttrCalled {
		t.Error("Should have called File.GetAttr")
	}

	err = os.Chmod(fn, 024)
	CheckSuccess(err)
	if fs.file.FileInfo.Mode&07777 != 024 {
		t.Error("chmod")
	}

	err = os.Chtimes(fn, 100, 101)
	CheckSuccess(err)
	if fs.file.FileInfo.Atime_ns != 100 || fs.file.FileInfo.Atime_ns != 101 {
		t.Error("Utimens")
	}

	// TODO - test chown if run as root.
}
