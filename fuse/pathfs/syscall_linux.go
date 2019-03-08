// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"bytes"
	"syscall"
	"unsafe"
)

var _zero uintptr

func getXAttr(path string, attr string, dest []byte) (value []byte, err error) {
	sz, err := syscall.Getxattr(path, attr, dest)
	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = syscall.Getxattr(path, attr, dest)
	}

	if err != nil {
		return nil, err
	}

	return dest[:sz], err
}

func listXAttr(path string) (attributes []string, err error) {
	dest := make([]byte, 0)
	sz, err := syscall.Listxattr(path, dest)
	if err != nil {
		return nil, err
	}

	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = syscall.Listxattr(path, dest)
	}

	if sz == 0 {
		return nil, err
	}

	// -1 to drop the final empty slice.
	dest = dest[:sz-1]
	attributesBytes := bytes.Split(dest, []byte{0})
	attributes = make([]string, len(attributesBytes))
	for i, v := range attributesBytes {
		attributes[i] = string(v)
	}
	return attributes, err
}

const _AT_SYMLINK_NOFOLLOW = 0x100

// Linux kernel syscall utimensat(2)
//
// Needed to implement SetAttr on symlinks correctly as only utimensat provides
// AT_SYMLINK_NOFOLLOW.
func sysUtimensat(dirfd int, pathname string, times *[2]syscall.Timespec, flags int) (err error) {

	// Null-terminated version of pathname
	p0, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return err
	}

	_, _, e1 := syscall.Syscall6(syscall.SYS_UTIMENSAT,
		uintptr(dirfd), uintptr(unsafe.Pointer(p0)), uintptr(unsafe.Pointer(times)), uintptr(flags), 0, 0)
	if e1 != 0 {
		err = syscall.Errno(e1)
	}
	return
}
