package fuse

import (
	"bytes"
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

func getxattr(path string, attr string, dest []byte) (sz int, errno int) {
	pathBs := syscall.StringBytePtr(path)
	attrBs := syscall.StringBytePtr(attr)
	size, _, errNo := syscall.Syscall6(
		syscall.SYS_GETXATTR,
		uintptr(unsafe.Pointer(pathBs)),
		uintptr(unsafe.Pointer(attrBs)),
		uintptr(unsafe.Pointer(&dest[0])),
		uintptr(len(dest)),
		0, 0)
	return int(size), int(errNo)
}

func GetXAttr(path string, attr string, dest []byte) (value []byte, errno int) {
	sz, errno := getxattr(path, attr, dest)

	for sz > cap(dest) && errno == 0 {
		dest = make([]byte, sz)
		sz, errno = getxattr(path, attr, dest)
	}

	if errno != 0 {
		return nil, errno
	}

	return dest[:sz], errno
}

func listxattr(path string, dest []byte) (sz int, errno int) {
	pathbs := syscall.StringBytePtr(path)
	size, _, errNo := syscall.Syscall(
		syscall.SYS_LISTXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(&dest[0])),
		uintptr(len(dest)))

	return int(size), int(errNo)
}

func ListXAttr(path string) (attributes []string, errno int) {
	dest := make([]byte, 1024)
	sz, errno := listxattr(path, dest)
	if errno != 0 {
		return nil, errno
	}

	for sz > cap(dest) && errno == 0 {
		dest = make([]byte, sz)
		sz, errno = listxattr(path, dest)
	}

	// -1 to drop the final empty slice.
	dest = dest[:sz-1]
	attributesBytes := bytes.Split(dest, []byte{0})
	attributes = make([]string, len(attributesBytes))
	for i, v := range attributesBytes {
		attributes[i] = string(v)
	}
	return attributes, errno
}

func Setxattr(path string, attr string, data []byte, flags int) (errno int) {
	pathbs := syscall.StringBytePtr(path)
	attrbs := syscall.StringBytePtr(attr)
	_, _, errNo := syscall.Syscall6(
		syscall.SYS_SETXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(flags), 0)

	return int(errNo)
}

func Removexattr(path string, attr string) (errno int) {
	pathbs := syscall.StringBytePtr(path)
	attrbs := syscall.StringBytePtr(attr)
	_, _, errNo := syscall.Syscall(
		syscall.SYS_REMOVEXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)), 0)
	return int(errNo)
}

func ioctl(fd int, cmd int, arg uintptr) (int, int) {
	r0, _, e1 := syscall.Syscall(
		syscall.SYS_IOCTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	val := int(r0)
	errno := int(e1)
	return val, errno
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
