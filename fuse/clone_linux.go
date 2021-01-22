package fuse

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	_IOC_READ = 2

	_IOC_NRBITS   = 8
	_IOC_TYPEBITS = 8
	_IOC_SIZEBITS = 14
	_IOC_DIRBITS  = 2

	_IOC_NRSHIFT   = 0
	_IOC_TYPESHIFT = (_IOC_NRSHIFT + _IOC_NRBITS)
	_IOC_SIZESHIFT = (_IOC_TYPESHIFT + _IOC_TYPEBITS)
	_IOC_DIRSHIFT  = (_IOC_SIZESHIFT + _IOC_SIZEBITS)
)

var (
	// include/uapi/linux/fuse.h
	// #define FUSE_DEV_IOC_CLONE	_IOR(229, 0, uint32_t)
	_FUSE_DEV_IOC_CLONE = _IOR(229, 0, 4)
)

func _IOR(typ, nr, size uint) uint {
	return _IOC(_IOC_READ, typ, nr, size)
}

func _IOC(dir, typ, nr, size uint) uint {
	return (dir << _IOC_DIRSHIFT) |
		(typ << _IOC_TYPESHIFT) |
		(nr << _IOC_NRSHIFT) |
		(size << _IOC_SIZESHIFT)
}

func cloneFuseConnection(sessionFd int) (int, error) {
	// Open a new connection and turn into a worker fd as described at
	// https://john-millikin.com/the-fuse-protocol
	workerFd, err := syscall.Open("/dev/fuse", os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}

	if err := unix.IoctlSetPointerInt(workerFd, _FUSE_DEV_IOC_CLONE, sessionFd); err != nil {
		return 0, err
	}

	return workerFd, nil
}
