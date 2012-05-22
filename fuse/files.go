package fuse

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

var _ = fmt.Println

// DataFile is for implementing read-only filesystems.  This
// assumes we already have the data in memory.
type DataFile struct {
	data []byte

	DefaultFile
}

var _ = (File)((*DataFile)(nil))

func (f *DataFile) String() string {
	l := len(f.data)
	if l > 10 {
		l = 10
	}

	return fmt.Sprintf("DataFile(%x)", f.data[:l])
}

func (f *DataFile) GetAttr(out *Attr) Status {
	out.Mode = S_IFREG | 0644
	out.Size = uint64(len(f.data))
	return OK
}

func NewDataFile(data []byte) *DataFile {
	f := new(DataFile)
	f.data = data
	return f
}

func (f *DataFile) Read(buf []byte, off int64) ([]byte, Status) {
	end := int(off) + int(len(buf))
	if end > len(f.data) {
		end = len(f.data)
	}

	return f.data[off:end], OK
}

////////////////

// DevNullFile accepts any write, and always returns EOF.
type DevNullFile struct {
	DefaultFile
}

var _ = (File)((*DevNullFile)(nil))

func NewDevNullFile() *DevNullFile {
	return new(DevNullFile)
}

func (f *DevNullFile) String() string {
	return "DevNullFile"
}

func (f *DevNullFile) Read(buf []byte, off int64) ([]byte, Status) {
	return nil, OK
}

func (f *DevNullFile) Write(content []byte, off int64) (uint32, Status) {
	return uint32(len(content)), OK
}

func (f *DevNullFile) Flush() Status {
	return OK
}

func (f *DevNullFile) Fsync(flags int) (code Status) {
	return OK
}

func (f *DevNullFile) Truncate(size uint64) (code Status) {
	return OK
}

////////////////

// LoopbackFile delegates all operations back to an underlying os.File.
type LoopbackFile struct {
	File *os.File

	DefaultFile
}

func (f *LoopbackFile) String() string {
	return fmt.Sprintf("LoopbackFile(%s)", f.File.Name())
}

func (f *LoopbackFile) Read(buf []byte, off int64) ([]byte, Status) {
	n, err := f.File.ReadAt(buf, off)
	if err == io.EOF {
		err = nil
	}
	return buf[:n], ToStatus(err)
}

func (f *LoopbackFile) Write(data []byte, off int64) (uint32, Status) {
	n, err := f.File.WriteAt(data, off)
	return uint32(n), ToStatus(err)
}

func (f *LoopbackFile) Release() {
	f.File.Close()
}

func (f *LoopbackFile) Flush() Status {
	return OK
}

func (f *LoopbackFile) Fsync(flags int) (code Status) {
	return ToStatus(syscall.Fsync(int(f.File.Fd())))
}

func (f *LoopbackFile) Truncate(size uint64) Status {
	return ToStatus(syscall.Ftruncate(int(f.File.Fd()), int64(size)))
}

// futimens missing from 6g runtime.

func (f *LoopbackFile) Chmod(mode uint32) Status {
	return ToStatus(f.File.Chmod(os.FileMode(mode)))
}

func (f *LoopbackFile) Chown(uid uint32, gid uint32) Status {
	return ToStatus(f.File.Chown(int(uid), int(gid)))
}

func (f *LoopbackFile) GetAttr(a *Attr) Status {
	st := syscall.Stat_t{}
	err := syscall.Fstat(int(f.File.Fd()), &st)
	if err != nil {
		return ToStatus(err)
	}
	a.FromStat(&st)
	return OK
}

////////////////////////////////////////////////////////////////

// ReadOnlyFile is a wrapper that denies writable operations
type ReadOnlyFile struct {
	File
}

func (f *ReadOnlyFile) String() string {
	return fmt.Sprintf("ReadOnlyFile(%s)", f.File.String())
}

func (f *ReadOnlyFile) Write(data []byte, off int64) (uint32, Status) {
	return 0, EPERM
}

func (f *ReadOnlyFile) Fsync(flag int) (code Status) {
	return OK
}

func (f *ReadOnlyFile) Truncate(size uint64) Status {
	return EPERM
}

func (f *ReadOnlyFile) Chmod(mode uint32) Status {
	return EPERM
}

func (f *ReadOnlyFile) Chown(uid uint32, gid uint32) Status {
	return EPERM
}
