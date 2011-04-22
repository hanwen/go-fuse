package fuse

import (
	"fmt"
	"os"
	"unsafe"
)

func (code Status) String() string {
	if code == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", int(code), os.Errno(code))
}

func replyString(opcode uint32, ptr unsafe.Pointer) string {
	var val interface{}
	switch opcode {
	case FUSE_LOOKUP:
		val = (*EntryOut)(ptr)
	case FUSE_OPEN:
		val = (*OpenOut)(ptr)
	}
	if val != nil {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func operationName(opcode uint32) string {
	switch opcode {
	case FUSE_LOOKUP:
		return "FUSE_LOOKUP"
	case FUSE_FORGET:
		return "FUSE_FORGET"
	case FUSE_GETATTR:
		return "FUSE_GETATTR"
	case FUSE_SETATTR:
		return "FUSE_SETATTR"
	case FUSE_READLINK:
		return "FUSE_READLINK"
	case FUSE_SYMLINK:
		return "FUSE_SYMLINK"
	case FUSE_MKNOD:
		return "FUSE_MKNOD"
	case FUSE_MKDIR:
		return "FUSE_MKDIR"
	case FUSE_UNLINK:
		return "FUSE_UNLINK"
	case FUSE_RMDIR:
		return "FUSE_RMDIR"
	case FUSE_RENAME:
		return "FUSE_RENAME"
	case FUSE_LINK:
		return "FUSE_LINK"
	case FUSE_OPEN:
		return "FUSE_OPEN"
	case FUSE_READ:
		return "FUSE_READ"
	case FUSE_WRITE:
		return "FUSE_WRITE"
	case FUSE_STATFS:
		return "FUSE_STATFS"
	case FUSE_RELEASE:
		return "FUSE_RELEASE"
	case FUSE_FSYNC:
		return "FUSE_FSYNC"
	case FUSE_SETXATTR:
		return "FUSE_SETXATTR"
	case FUSE_GETXATTR:
		return "FUSE_GETXATTR"
	case FUSE_LISTXATTR:
		return "FUSE_LISTXATTR"
	case FUSE_REMOVEXATTR:
		return "FUSE_REMOVEXATTR"
	case FUSE_FLUSH:
		return "FUSE_FLUSH"
	case FUSE_INIT:
		return "FUSE_INIT"
	case FUSE_OPENDIR:
		return "FUSE_OPENDIR"
	case FUSE_READDIR:
		return "FUSE_READDIR"
	case FUSE_RELEASEDIR:
		return "FUSE_RELEASEDIR"
	case FUSE_FSYNCDIR:
		return "FUSE_FSYNCDIR"
	case FUSE_GETLK:
		return "FUSE_GETLK"
	case FUSE_SETLK:
		return "FUSE_SETLK"
	case FUSE_SETLKW:
		return "FUSE_SETLKW"
	case FUSE_ACCESS:
		return "FUSE_ACCESS"
	case FUSE_CREATE:
		return "FUSE_CREATE"
	case FUSE_INTERRUPT:
		return "FUSE_INTERRUPT"
	case FUSE_BMAP:
		return "FUSE_BMAP"
	case FUSE_DESTROY:
		return "FUSE_DESTROY"
	case FUSE_IOCTL:
		return "FUSE_IOCTL"
	case FUSE_POLL:
		return "FUSE_POLL"
	}
	return "UNKNOWN"
}



var inputSizeMap map[int]int
var outputSizeMap map[int]int

func init() {
	inputSizeMap = map[int]int{
		FUSE_LOOKUP:      0,
		FUSE_FORGET:      unsafe.Sizeof(ForgetIn{}),
		FUSE_GETATTR:     unsafe.Sizeof(GetAttrIn{}),
		FUSE_SETATTR:     unsafe.Sizeof(SetAttrIn{}),
		FUSE_READLINK:    0,
		FUSE_SYMLINK:     0,
		FUSE_MKNOD:       unsafe.Sizeof(MknodIn{}),
		FUSE_MKDIR:       unsafe.Sizeof(MkdirIn{}),
		FUSE_UNLINK:      0,
		FUSE_RMDIR:       0,
		FUSE_RENAME:      unsafe.Sizeof(RenameIn{}),
		FUSE_LINK:        unsafe.Sizeof(LinkIn{}),
		FUSE_OPEN:        unsafe.Sizeof(OpenIn{}),
		FUSE_READ:        unsafe.Sizeof(ReadIn{}),
		FUSE_WRITE:       unsafe.Sizeof(WriteIn{}),
		FUSE_STATFS:      0,
		FUSE_RELEASE:     unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNC:       unsafe.Sizeof(FsyncIn{}),
		FUSE_SETXATTR:    unsafe.Sizeof(SetXAttrIn{}),
		FUSE_GETXATTR:    unsafe.Sizeof(GetXAttrIn{}),
		FUSE_LISTXATTR:   unsafe.Sizeof(GetXAttrIn{}),
		FUSE_REMOVEXATTR: 0,
		FUSE_FLUSH:       unsafe.Sizeof(FlushIn{}),
		FUSE_INIT:        unsafe.Sizeof(InitIn{}),
		FUSE_OPENDIR:     unsafe.Sizeof(OpenIn{}),
		FUSE_READDIR:     unsafe.Sizeof(ReadIn{}),
		FUSE_RELEASEDIR:  unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNCDIR:    unsafe.Sizeof(FsyncIn{}),
		FUSE_GETLK:       0,
		FUSE_SETLK:       0,
		FUSE_SETLKW:      0,
		FUSE_ACCESS:      unsafe.Sizeof(AccessIn{}),
		FUSE_CREATE:      unsafe.Sizeof(CreateIn{}),
		FUSE_INTERRUPT:   unsafe.Sizeof(InterruptIn{}),
		FUSE_BMAP:        unsafe.Sizeof(BmapIn{}),
		FUSE_DESTROY:     0,
		FUSE_IOCTL:       unsafe.Sizeof(IoctlIn{}),
		FUSE_POLL:        unsafe.Sizeof(PollIn{}),
	}

	outputSizeMap = map[int]int{
		FUSE_LOOKUP:      unsafe.Sizeof(EntryOut{}),
		FUSE_FORGET:      0,
		FUSE_GETATTR:     unsafe.Sizeof(AttrOut{}),
		FUSE_SETATTR:     unsafe.Sizeof(AttrOut{}),
		FUSE_READLINK:    0,
		FUSE_SYMLINK:     unsafe.Sizeof(EntryOut{}),
		FUSE_MKNOD:       unsafe.Sizeof(EntryOut{}),
		FUSE_MKDIR:       unsafe.Sizeof(EntryOut{}),
		FUSE_UNLINK:      0,
		FUSE_RMDIR:       0,
		FUSE_RENAME:      0,
		FUSE_LINK:        unsafe.Sizeof(EntryOut{}),
		FUSE_OPEN:        unsafe.Sizeof(OpenOut{}),
		FUSE_READ:        0,
		FUSE_WRITE:       unsafe.Sizeof(WriteOut{}),
		FUSE_STATFS:      unsafe.Sizeof(StatfsOut{}),
		FUSE_RELEASE:     0,
		FUSE_FSYNC:       0,
		FUSE_SETXATTR:    0,
		FUSE_GETXATTR:    unsafe.Sizeof(GetXAttrOut{}),
		FUSE_LISTXATTR:   unsafe.Sizeof(GetXAttrOut{}),
		FUSE_REMOVEXATTR: 0,
		FUSE_FLUSH:       0,
		FUSE_INIT:        unsafe.Sizeof(InitOut{}),
		FUSE_OPENDIR:     unsafe.Sizeof(OpenOut{}),
		FUSE_READDIR:     0,
		FUSE_RELEASEDIR:  0,
		FUSE_FSYNCDIR:    0,
		// TODO
		FUSE_GETLK:     0,
		FUSE_SETLK:     0,
		FUSE_SETLKW:    0,
		FUSE_ACCESS:    0,
		FUSE_CREATE:    unsafe.Sizeof(CreateOut{}),
		FUSE_INTERRUPT: 0,
		FUSE_BMAP:      unsafe.Sizeof(BmapOut{}),
		FUSE_DESTROY:   0,
		FUSE_IOCTL:     unsafe.Sizeof(IoctlOut{}),
		FUSE_POLL:      unsafe.Sizeof(PollOut{}),
	}
}
