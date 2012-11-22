package fuse

import (
	"fmt"
	"os"
	"sync"
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

func (f *DataFile) Read(buf []byte, off int64) (res ReadResult, code Status) {
	end := int(off) + int(len(buf))
	if end > len(f.data) {
		end = len(f.data)
	}

	return &ReadResultData{f.data[off:end]}, OK
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

func (f *DevNullFile) Read(buf []byte, off int64) (ReadResult, Status) {
	return &ReadResultData{}, OK
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

	// os.File is not threadsafe. Although fd themselves are
	// constant during the lifetime of an open file, the OS may
	// reuse the fd number after it is closed. When combined with
	// threads,
	lock sync.Mutex
	DefaultFile
}

func (f *LoopbackFile) String() string {
	return fmt.Sprintf("LoopbackFile(%s)", f.File.Name())
}

func (f *LoopbackFile) Read(buf []byte, off int64) (res ReadResult, code Status) {
	f.lock.Lock()
	r := &ReadResultFd{
		Fd:  f.File.Fd(),
		Off: off,
		Sz:  len(buf),
	}
	f.lock.Unlock()
	return r, OK
}

func (f *LoopbackFile) Write(data []byte, off int64) (uint32, Status) {
	f.lock.Lock()
	n, err := f.File.WriteAt(data, off)
	f.lock.Unlock()
	return uint32(n), ToStatus(err)
}

func (f *LoopbackFile) Release() {
	f.lock.Lock()
	f.File.Close()
	f.lock.Unlock()
}

func (f *LoopbackFile) Flush() Status {
	f.lock.Lock()

	// Since Flush() may be called for each dup'd fd, we don't
	// want to really close the file, we just want to flush. This
	// is achieved by closing a dup'd fd.
	newFd, err := syscall.Dup(int(f.File.Fd()))
	f.lock.Unlock()

	if err != nil {
		return ToStatus(err)
	}
	err = syscall.Close(newFd)
	return ToStatus(err)
}

func (f *LoopbackFile) Fsync(flags int) (code Status) {
	f.lock.Lock()
	r := ToStatus(syscall.Fsync(int(f.File.Fd())))
	f.lock.Unlock()

	return r
}

func (f *LoopbackFile) Truncate(size uint64) Status {
	f.lock.Lock()
	r := ToStatus(syscall.Ftruncate(int(f.File.Fd()), int64(size)))
	f.lock.Unlock()

	return r
}

// futimens missing from 6g runtime.

func (f *LoopbackFile) Chmod(mode uint32) Status {
	f.lock.Lock()
	r := ToStatus(f.File.Chmod(os.FileMode(mode)))
	f.lock.Unlock()

	return r
}

func (f *LoopbackFile) Chown(uid uint32, gid uint32) Status {
	f.lock.Lock()
	r := ToStatus(f.File.Chown(int(uid), int(gid)))
	f.lock.Unlock()

	return r
}

func (f *LoopbackFile) GetAttr(a *Attr) Status {
	st := syscall.Stat_t{}
	f.lock.Lock()
	err := syscall.Fstat(int(f.File.Fd()), &st)
	f.lock.Unlock()
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
