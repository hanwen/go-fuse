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

func (f *DataFile) String() string {
	l := len(f.data)
	if l > 10 {
		l = 10
	}

	return fmt.Sprintf("DataFile(%x)", f.data[:l])
}

func (f *DataFile) GetAttr() (*Attr, Status) {
	return &Attr{Mode: S_IFREG | 0644, Size: uint64(len(f.data))}, OK
}

func NewDataFile(data []byte) *DataFile {
	f := new(DataFile)
	f.data = data
	return f
}

func (f *DataFile) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	end := int(input.Offset) + int(input.Size)
	if end > len(f.data) {
		end = len(f.data)
	}

	return f.data[input.Offset:end], OK
}

////////////////

// DevNullFile accepts any write, and always returns EOF.
type DevNullFile struct {
	DefaultFile
}

func NewDevNullFile() *DevNullFile {
	return new(DevNullFile)
}

func (f *DevNullFile) String() string {
	return "DevNullFile"
}

func (f *DevNullFile) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	return []byte{}, OK
}

func (f *DevNullFile) Write(input *WriteIn, content []byte) (uint32, Status) {
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

func (f *LoopbackFile) Read(input *ReadIn, buffers BufferPool) ([]byte, Status) {
	slice := buffers.AllocBuffer(input.Size)

	n, err := f.File.ReadAt(slice, int64(input.Offset))
	if err == io.EOF {
		err = nil
	}
	return slice[:n], ToStatus(err)
}

func (f *LoopbackFile) Write(input *WriteIn, data []byte) (uint32, Status) {
	n, err := f.File.WriteAt(data, int64(input.Offset))
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

func (f *LoopbackFile) GetAttr() (*Attr, Status) {
	st := syscall.Stat_t{}
	err := syscall.Fstat(int(f.File.Fd()), &st)
	if err != nil {
		return nil, ToStatus(err)
	}
	a := &Attr{}
	a.FromStat(&st)
	return a, OK
}

////////////////////////////////////////////////////////////////

// ReadOnlyFile is a wrapper that denies writable operations
type ReadOnlyFile struct {
	File
}

func (f *ReadOnlyFile) String() string {
	return fmt.Sprintf("ReadOnlyFile(%s)", f.File.String())
}

func (f *ReadOnlyFile) Write(input *WriteIn, data []byte) (uint32, Status) {
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
