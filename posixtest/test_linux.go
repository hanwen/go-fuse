package posixtest

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func sysFcntlFlockGetOFDLock(fd uintptr, lk *syscall.Flock_t) error {
	return syscall.FcntlFlock(fd, unix.F_OFD_GETLK, lk)
}
