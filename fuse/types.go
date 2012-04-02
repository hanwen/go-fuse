package fuse

import (
	"os"
	"syscall"
	"github.com/hanwen/go-fuse/raw"
)

const (
	S_IFDIR = syscall.S_IFDIR
	S_IFREG = syscall.S_IFREG
	S_IFLNK = syscall.S_IFLNK
	S_IFIFO = syscall.S_IFIFO

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


type Attr raw.Attr

type Owner raw.Owner

type Context raw.Context

type StatfsOut raw.StatfsOut

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

