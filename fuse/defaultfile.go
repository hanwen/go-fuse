package fuse

import (
	"log"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

var _ = (File)((*DefaultFile)(nil))

func (f *DefaultFile) SetInode(*Inode) {
}

func (f *DefaultFile) InnerFile() File {
	return nil
}

func (f *DefaultFile) String() string {
	return "DefaultFile"
}

func (f *DefaultFile) Read(buf []byte, off int64) (ReadResult, Status) {
	return &ReadResultData{}, ENOSYS
}

func (f *DefaultFile) Write(data []byte, off int64) (uint32, Status) {
	return 0, ENOSYS
}

func (f *DefaultFile) Flush() Status {
	return OK
}

func (f *DefaultFile) Release() {

}

func (f *DefaultFile) GetAttr(*Attr) Status {
	return ENOSYS
}

func (f *DefaultFile) Fsync(flags int) (code Status) {
	return ENOSYS
}

func (f *DefaultFile) Utimens(atime *time.Time, mtime *time.Time) Status {
	return ENOSYS
}

func (f *DefaultFile) Truncate(size uint64) Status {
	return ENOSYS
}

func (f *DefaultFile) Chown(uid uint32, gid uint32) Status {
	return ENOSYS
}

func (f *DefaultFile) Chmod(perms uint32) Status {
	return ENOSYS
}

func (f *DefaultFile) Ioctl(input *raw.IoctlIn) (output *raw.IoctlOut, data []byte, code Status) {
	return nil, nil, ENOSYS
}
