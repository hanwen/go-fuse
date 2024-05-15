package posixtest

import "syscall"

// Exist at least from macOS Siera 10.12
const sys_F_OFD_GETLK = 92

func sysFcntlFlockGetOFDLock(fd uintptr, lk *syscall.Flock_t) error {
	return syscall.FcntlFlock(fd, sys_F_OFD_GETLK, lk)
}
