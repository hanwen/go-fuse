package fuse

import (
	"fmt"
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

func (me *DataFile) String() string {
	l := len(me.data)
	if l > 10 {
		l = 10
	}

	return fmt.Sprintf("DataFile(%x)", me.data[:l])
}

func (me *DataFile) GetAttr() (*os.FileInfo, Status) {
	return &os.FileInfo{Mode: S_IFREG | 0644, Size: int64(len(me.data))}, OK
}

func NewDataFile(data []byte) *DataFile {
	f := new(DataFile)
	f.data = data
	return f
}

func (me *DataFile) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	end := int(input.Offset) + int(input.Size)
	if end > len(me.data) {
		end = len(me.data)
	}

	return me.data[input.Offset:end], OK
}

////////////////

// DevNullFile accepts any write, and always returns EOF.
type DevNullFile struct {
	DefaultFile
}

func NewDevNullFile() *DevNullFile {
	return new(DevNullFile)
}

func (me *DevNullFile) String() string {
	return "DevNullFile"
}

func (me *DevNullFile) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	return []byte{}, OK
}

func (me *DevNullFile) Write(input *WriteIn, content []byte) (uint32, Status) {
	return uint32(len(content)), OK
}

func (me *DevNullFile) Flush() Status {
	return OK
}

func (me *DevNullFile) Fsync(*FsyncIn) (code Status) {
	return OK
}

func (me *DevNullFile) Truncate(size uint64) (code Status) {
	return OK
}

////////////////

// LoopbackFile delegates all operations back to an underlying os.File.
type LoopbackFile struct {
	File *os.File

	DefaultFile
}

func (me *LoopbackFile) String() string {
	return fmt.Sprintf("LoopbackFile(%s)", me.File.Name())
}

func (me *LoopbackFile) Read(input *ReadIn, buffers BufferPool) ([]byte, Status) {
	slice := buffers.AllocBuffer(input.Size)

	n, err := me.File.ReadAt(slice, int64(input.Offset))
	// TODO - fix Go ndocumentation.
	if err == os.EOF {
		err = nil
	}
	return slice[:n], OsErrorToErrno(err)
}

func (me *LoopbackFile) Write(input *WriteIn, data []byte) (uint32, Status) {
	n, err := me.File.WriteAt(data, int64(input.Offset))
	return uint32(n), OsErrorToErrno(err)
}

func (me *LoopbackFile) Release() {
	me.File.Close()
}

func (me *LoopbackFile) Fsync(*FsyncIn) (code Status) {
	return Status(syscall.Fsync(me.File.Fd()))
}

func (me *LoopbackFile) Truncate(size uint64) Status {
	return Status(syscall.Ftruncate(me.File.Fd(), int64(size)))
}

// futimens missing from 6g runtime.

func (me *LoopbackFile) Chmod(mode uint32) Status {
	return OsErrorToErrno(me.File.Chmod(mode))
}

func (me *LoopbackFile) Chown(uid uint32, gid uint32) Status {
	return OsErrorToErrno(me.File.Chown(int(uid), int(gid)))
}

func (me *LoopbackFile) GetAttr() (*os.FileInfo, Status) {
	fi, err := me.File.Stat()
	if err != nil {
		return nil, OsErrorToErrno(err)
	}
	return fi, OK
}

////////////////////////////////////////////////////////////////

// ReadOnlyFile is a wrapper that denies writable operations
type ReadOnlyFile struct {
	File
}

func (me *ReadOnlyFile) String() string {
	return fmt.Sprintf("ReadOnlyFile(%s)", me.File.String())
}

func (me *ReadOnlyFile) Write(input *WriteIn, data []byte) (uint32, Status) {
	return 0, EPERM
}

func (me *ReadOnlyFile) Fsync(*FsyncIn) (code Status) {
	return OK
}

func (me *ReadOnlyFile) Truncate(size uint64) Status {
	return EPERM
}

func (me *ReadOnlyFile) Chmod(mode uint32) Status {
	return EPERM
}

func (me *ReadOnlyFile) Chown(uid uint32, gid uint32) Status {
	return EPERM
}
