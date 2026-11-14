// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// On Windows, the FUSE protocol is served in-process (see package
// winfsp), so the wire format follows Linux.

const (
	ENODATA = syscall.ENODATA
	ENOATTR = syscall.ENODATA

	EREMOTEIO = syscall.EIO
)

const (
	CAP_NO_OPENDIR_SUPPORT  = (1 << 24)
	CAP_EXPLICIT_INVAL_DATA = (1 << 25)

	CAP_MAP_ALIGNMENT      = (1 << 26)
	CAP_SUBMOUNTS          = (1 << 27)
	CAP_HANDLE_KILLPRIV_V2 = (1 << 28)
	CAP_SETXATTR_EXT       = (1 << 29)
	CAP_INIT_EXT           = (1 << 30)
	CAP_INIT_RESERVED      = (1 << 31)

	CAP_RENAME_SWAP = 0x0
)

func (o *InitOut) setFlags(flags uint64) {
	o.Flags = uint32(flags) | CAP_INIT_EXT
	o.Flags2 = uint32(flags >> 32)
}

// Linux values, as used on the wire.
const (
	syscall_O_DIRECTORY = 0x10000
	syscall_O_LARGEFILE = 0x8000
	syscall_O_NOATIME   = 0x40000

	_UTIME_OMIT = (1 << 30) - 2
)

const outputDataSize = 288

const inputDataSize = 128

const (
	_FUSE_KERNEL_VERSION   = 7
	_MINIMUM_MINOR_VERSION = 12
	_OUR_MINOR_VERSION     = 47
)

// dirent is linux_dirent64, without the name.
type dirent struct {
	Ino    uint64
	Off    int64
	Reclen uint16
	Type   uint8
	Name   [1]uint8
}

func (de *dirent) nameLength() int {
	return int(de.Reclen) - 19
}

// fdCall is only used by Server, which does not exist on Windows.
type fdCall struct{}

func getMaxWrite() int {
	return MAX_KERNEL_WRITE
}

// MAX_KERNEL_WRITE is the largest write that ProtocolServer accepts.
const MAX_KERNEL_WRITE = 1024 * 1024

// Bytes reads from the file handle in r.Fd at r.Off.
func (r *readResultFd) Bytes(buf []byte) ([]byte, syscall.Errno) {
	sz := min(len(buf), r.Sz)
	if sz == 0 {
		return buf[:0], OK
	}
	var ov windows.Overlapped
	ov.Offset = uint32(r.Off)
	ov.OffsetHigh = uint32(r.Off >> 32)
	var n uint32
	err := windows.ReadFile(windows.Handle(r.Fd), buf[:sz], &n, &ov)
	if err == windows.ERROR_HANDLE_EOF {
		err = nil
	}
	return buf[:n], ToErrno(err)
}
