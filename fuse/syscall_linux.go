package fuse

import (
	"os"
	"syscall"
	"unsafe"
)

// TODO - move these into Go's syscall package.

func writev(fd int, iovecs *syscall.Iovec, cnt int) (n int, errno int) {
	n1, _, e1 := syscall.Syscall(
		syscall.SYS_WRITEV,
		uintptr(fd), uintptr(unsafe.Pointer(iovecs)), uintptr(cnt))
	return int(n1), int(e1)
}

func Writev(fd int, packet [][]byte) (n int, err error) {
	iovecs := make([]syscall.Iovec, 0, len(packet))

	for _, v := range packet {
		if len(v) == 0 {
			continue
		}
		vec := syscall.Iovec{
			Base: &v[0],
		}
		vec.SetLen(len(v))
		iovecs = append(iovecs, vec)
	}

	n, errno := writev(fd, &iovecs[0], len(iovecs))
	if errno != 0 {
		err = os.NewSyscallError("writev", syscall.Errno(errno))
	}
	return n, err
}

const AT_FDCWD = -100

func Linkat(fd1 int, n1 string, fd2 int, n2 string) int {
	b1 := syscall.StringBytePtr(n1)
	b2 := syscall.StringBytePtr(n2)

	_, _, errNo := syscall.Syscall6(
		syscall.SYS_LINKAT,
		uintptr(fd1),
		uintptr(unsafe.Pointer(b1)),
		uintptr(fd2),
		uintptr(unsafe.Pointer(b2)),
		0, 0)
	return int(errNo)
}
