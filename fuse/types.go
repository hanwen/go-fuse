package fuse

import (
	"os"
	"syscall"
	"github.com/hanwen/go-fuse/raw"
)

const (
	FUSE_ROOT_ID = 1

	FUSE_UNKNOWN_INO = 0xffffffff

	CUSE_UNRESTRICTED_IOCTL = (1 << 0)

	FUSE_LK_FLOCK = (1 << 0)

	FUSE_IOCTL_MAX_IOV = 256

	FUSE_POLL_SCHEDULE_NOTIFY = (1 << 0)

	CUSE_INIT_INFO_MAX = 4096

	S_IFDIR = syscall.S_IFDIR
	S_IFREG = syscall.S_IFREG
	S_IFLNK = syscall.S_IFLNK
	S_IFIFO = syscall.S_IFIFO

	CUSE_INIT = 4096

	O_ANYWRITE = uint32(os.O_WRONLY | os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_TRUNC)
)

const PAGESIZE = 4096

const (
	_DEFAULT_BACKGROUND_TASKS = 12
)

type Status int32

const (
	OK      = Status(0)
	EACCES  = Status(syscall.EACCES)
	EBUSY   = Status(syscall.EBUSY)
	EINVAL  = Status(syscall.EINVAL)
	EIO     = Status(syscall.EIO)
	ENOENT  = Status(syscall.ENOENT)
	ENOSYS  = Status(syscall.ENOSYS)
	ENODATA = Status(syscall.ENODATA)
	ENOTDIR = Status(syscall.ENOTDIR)
	EPERM   = Status(syscall.EPERM)
	ERANGE  = Status(syscall.ERANGE)
	EXDEV   = Status(syscall.EXDEV)
	EBADF   = Status(syscall.EBADF)
	ENODEV  = Status(syscall.ENODEV)
	EROFS   = Status(syscall.EROFS)
)

type NotifyCode int

const (
	NOTIFY_POLL        = -1
	NOTIFY_INVAL_INODE = -2
	NOTIFY_INVAL_ENTRY = -3
	NOTIFY_CODE_MAX    = -4
)

type Attr struct {
	Ino       uint64
	Size      uint64
	Blocks    uint64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Atimensec uint32
	Mtimensec uint32
	Ctimensec uint32
	Mode      uint32
	Nlink     uint32
	Owner
	Rdev    uint32
	Blksize uint32
	Padding uint32
}

type Owner struct {
	Uid uint32
	Gid uint32
}

type Context struct {
	Owner
	Pid uint32
}

type Kstatfs struct {
	Blocks  uint64
	Bfree   uint64
	Bavail  uint64
	Files   uint64
	Ffree   uint64
	Bsize   uint32
	NameLen uint32
	Frsize  uint32
	Padding uint32
	Spare   [6]uint32
}

type EntryOut struct {
	NodeId         uint64
	Generation     uint64
	EntryValid     uint64
	AttrValid      uint64
	EntryValidNsec uint32
	AttrValidNsec  uint32
	Attr
}

type AttrOut struct {
	AttrValid     uint64
	AttrValidNsec uint32
	Dummy         uint32
	Attr
}

type CreateOut struct {
	EntryOut
	raw.OpenOut
}

type FlushIn struct {
	Fh        uint64
	Unused    uint32
	Padding   uint32
	LockOwner uint64
}

const (
	READ_LOCKOWNER = (1 << 1)
)

type ReadIn struct {
	Fh        uint64
	Offset    uint64
	Size      uint32
	ReadFlags uint32
	LockOwner uint64
	Flags     uint32
	Padding   uint32
}

const (
	WRITE_CACHE     = (1 << 0)
	WRITE_LOCKOWNER = (1 << 1)
)

type WriteIn struct {
	Fh         uint64
	Offset     uint64
	Size       uint32
	WriteFlags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

type StatfsOut struct {
	Kstatfs
}

type InHeader struct {
	Length uint32
	opcode
	Unique uint64
	NodeId uint64
	Context
	Padding uint32
}

type Dirent struct {
	Ino     uint64
	Off     uint64
	NameLen uint32
	Typ     uint32
}
