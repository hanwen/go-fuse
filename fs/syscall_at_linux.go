// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Raw wrappers for fchmodat2 (Linux 6.6) and the *xattrat calls
// (Linux 6.13), which x/sys/unix does not wrap, or wraps in a way
// that hides ENOSYS.

func errnoErr(e unix.Errno) error {
	if e == 0 {
		return nil
	}
	return e
}

func fchmodat2(dirfd int, name string, mode uint32, flags int) error {
	p, err := unix.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, e := unix.Syscall6(unix.SYS_FCHMODAT2, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(mode), uintptr(flags), 0, 0)
	return errnoErr(e)
}

// xattrArgs is struct xattr_args.
type xattrArgs struct {
	Value uint64
	Size  uint32
	Flags uint32
}

func bufAddr(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(uintptr(unsafe.Pointer(&b[0])))
}

func getxattrat(dirfd int, name string, atFlags int, attr string, dest []byte) (int, error) {
	p, err := unix.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	a, err := unix.BytePtrFromString(attr)
	if err != nil {
		return 0, err
	}
	args := xattrArgs{Value: bufAddr(dest), Size: uint32(len(dest))}
	r, _, e := unix.Syscall6(unix.SYS_GETXATTRAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(atFlags),
		uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&args)), unsafe.Sizeof(args))
	runtime.KeepAlive(dest)
	return int(r), errnoErr(e)
}

func setxattrat(dirfd int, name string, atFlags int, attr string, data []byte, flags int) error {
	p, err := unix.BytePtrFromString(name)
	if err != nil {
		return err
	}
	a, err := unix.BytePtrFromString(attr)
	if err != nil {
		return err
	}
	args := xattrArgs{Value: bufAddr(data), Size: uint32(len(data)), Flags: uint32(flags)}
	_, _, e := unix.Syscall6(unix.SYS_SETXATTRAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(atFlags),
		uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&args)), unsafe.Sizeof(args))
	runtime.KeepAlive(data)
	return errnoErr(e)
}

func listxattrat(dirfd int, name string, atFlags int, dest []byte) (int, error) {
	p, err := unix.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	var d unsafe.Pointer
	if len(dest) > 0 {
		d = unsafe.Pointer(&dest[0])
	}
	r, _, e := unix.Syscall6(unix.SYS_LISTXATTRAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(atFlags),
		uintptr(d), uintptr(len(dest)), 0)
	return int(r), errnoErr(e)
}

func removexattrat(dirfd int, name string, atFlags int, attr string) error {
	p, err := unix.BytePtrFromString(name)
	if err != nil {
		return err
	}
	a, err := unix.BytePtrFromString(attr)
	if err != nil {
		return err
	}
	_, _, e := unix.Syscall6(unix.SYS_REMOVEXATTRAT, uintptr(dirfd), uintptr(unsafe.Pointer(p)), uintptr(atFlags),
		uintptr(unsafe.Pointer(a)), 0, 0)
	return errnoErr(e)
}
