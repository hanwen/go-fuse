package fuse

import (
	"log"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

var _ = (File)((*defaultFile)(nil))

type defaultFile struct{}

// DefaultFile returns ENOSYS for every operation.
func NewDefaultFile() File {
	return &defaultFile{}
}

func (f *defaultFile) SetInode(*Inode) {
}

func (f *defaultFile) InnerFile() File {
	return nil
}

func (f *defaultFile) String() string {
	return "defaultFile"
}

func (f *defaultFile) Read(buf []byte, off int64) (ReadResult, Status) {
	return nil, ENOSYS
}

func (f *defaultFile) Write(data []byte, off int64) (uint32, Status) {
	return 0, ENOSYS
}

func (f *defaultFile) Flush() Status {
	return OK
}

func (f *defaultFile) Release() {

}

func (f *defaultFile) GetAttr(*Attr) Status {
	return ENOSYS
}

func (f *defaultFile) Fsync(flags int) (code Status) {
	return ENOSYS
}

func (f *defaultFile) Utimens(atime *time.Time, mtime *time.Time) Status {
	return ENOSYS
}

func (f *defaultFile) Truncate(size uint64) Status {
	return ENOSYS
}

func (f *defaultFile) Chown(uid uint32, gid uint32) Status {
	return ENOSYS
}

func (f *defaultFile) Chmod(perms uint32) Status {
	return ENOSYS
}

func (f *defaultFile) Ioctl(input *raw.IoctlIn) (output *raw.IoctlOut, data []byte, code Status) {
	return nil, nil, ENOSYS
}

func (f *defaultFile) Allocate(off uint64, size uint64, mode uint32) (code Status) {
	return ENOSYS
}
