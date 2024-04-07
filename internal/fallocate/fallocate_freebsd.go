package fallocate

import (
	"golang.org/x/sys/unix"
)

func fallocate(fd int, mode uint32, off int64, len int64) error {
	// Ignore mode
	_ = mode
	_, _, errno := unix.Syscall(unix.SYS_POSIX_FALLOCATE, uintptr(fd), uintptr(off), uintptr(len))
	if errno != 0 {
		return errno
	}
	return nil
}
