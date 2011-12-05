package fuse

import (
	"log"
)

var _ = log.Println

func (me *DefaultFile) SetInode(*Inode) {
}

func (me *DefaultFile) InnerFile() File {
	return nil
}

func (me *DefaultFile) String() string {
	return "DefaultFile"
}

func (me *DefaultFile) Read(*ReadIn, BufferPool) ([]byte, Status) {
	return []byte(""), ENOSYS
}

func (me *DefaultFile) Write(*WriteIn, []byte) (uint32, Status) {
	return 0, ENOSYS
}

func (me *DefaultFile) Flush() Status {
	return OK
}

func (me *DefaultFile) Release() {

}

func (me *DefaultFile) GetAttr() (*Attr, Status) {
	return nil, ENOSYS
}

func (me *DefaultFile) Fsync(*FsyncIn) (code Status) {
	return ENOSYS
}

func (me *DefaultFile) Utimens(atimeNs int64, mtimeNs int64) Status {
	return ENOSYS
}

func (me *DefaultFile) Truncate(size uint64) Status {
	return ENOSYS
}

func (me *DefaultFile) Chown(uid uint32, gid uint32) Status {
	return ENOSYS
}

func (me *DefaultFile) Chmod(perms uint32) Status {
	return ENOSYS
}

func (me *DefaultFile) Ioctl(input *IoctlIn) (output *IoctlOut, data []byte, code Status) {
	return nil, nil, ENOSYS
}
