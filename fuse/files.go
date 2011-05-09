package fuse
import (
	"os"
	"syscall"
)

// ReadOnlyFile is for implementing read-only filesystems.  This
// assumes we already have the data in memory.
type ReadOnlyFile struct {
	data []byte

	DefaultFile
}

func NewReadOnlyFile(data []byte) *ReadOnlyFile {
	f := new(ReadOnlyFile)
	f.data = data
	return f
}

func (me *ReadOnlyFile) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
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

////////////////

// LoopbackFile delegates all operations back to an underlying os.File.
type LoopbackFile struct {
	file *os.File

	DefaultFile
}

func (me *LoopbackFile) Read(input *ReadIn, buffers BufferPool) ([]byte, Status) {
	slice := buffers.AllocBuffer(input.Size)

	n, err := me.file.ReadAt(slice, int64(input.Offset))
	if err == os.EOF {
		return slice[:n], OK
	}
	return slice[:n], OsErrorToErrno(err)
}

func (me *LoopbackFile) Write(input *WriteIn, data []byte) (uint32, Status) {
	n, err := me.file.WriteAt(data, int64(input.Offset))
	return uint32(n), OsErrorToErrno(err)
}

func (me *LoopbackFile) Release() {
	me.file.Close()
}

func (me *LoopbackFile) Fsync(*FsyncIn) (code Status) {
	return Status(syscall.Fsync(me.file.Fd()))
}


func (me *LoopbackFile) Truncate(size uint64) Status {
	return Status(syscall.Ftruncate(me.file.Fd(), int64(size)))
}

// futimens missing from 6g runtime.

func (me *LoopbackFile) Chmod(mode uint32) Status {
	return OsErrorToErrno(me.file.Chmod(mode))
}

func (me *LoopbackFile) Chown(uid uint32, gid uint32) Status {
	return OsErrorToErrno(me.file.Chown(int(uid), int(gid)))
}

func (me *LoopbackFile) GetAttr() (*os.FileInfo, Status) {
	fi, err := me.file.Stat()
	if err != nil {
		return nil, OsErrorToErrno(err)
	}
	return fi, OK
}

