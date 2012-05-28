package fuse

import (
	"github.com/hanwen/go-fuse/raw"
	"os"
	"syscall"
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
