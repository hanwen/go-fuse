// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"fmt"
	"syscall"
)

// ntStatus is a Windows NTSTATUS code.
type ntStatus uint32

// NTSTATUS values, from ntstatus.h.
const (
	_STATUS_SUCCESS                 ntStatus = 0x00000000
	_STATUS_REPARSE                 ntStatus = 0x00000104
	_STATUS_BUFFER_OVERFLOW         ntStatus = 0x80000005
	_STATUS_DEVICE_BUSY             ntStatus = 0x80000011
	_STATUS_UNSUCCESSFUL            ntStatus = 0xC0000001
	_STATUS_NOT_IMPLEMENTED         ntStatus = 0xC0000002
	_STATUS_INVALID_HANDLE          ntStatus = 0xC0000008
	_STATUS_INVALID_PARAMETER       ntStatus = 0xC000000D
	_STATUS_NO_SUCH_DEVICE          ntStatus = 0xC000000E
	_STATUS_INVALID_DEVICE_REQUEST  ntStatus = 0xC0000010
	_STATUS_END_OF_FILE             ntStatus = 0xC0000011
	_STATUS_ACCESS_DENIED           ntStatus = 0xC0000022
	_STATUS_BUFFER_TOO_SMALL        ntStatus = 0xC0000023
	_STATUS_OBJECT_NAME_INVALID     ntStatus = 0xC0000033
	_STATUS_OBJECT_NAME_NOT_FOUND   ntStatus = 0xC0000034
	_STATUS_OBJECT_NAME_COLLISION   ntStatus = 0xC0000035
	_STATUS_OBJECT_PATH_NOT_FOUND   ntStatus = 0xC000003A
	_STATUS_SHARING_VIOLATION       ntStatus = 0xC0000043
	_STATUS_EAS_NOT_SUPPORTED       ntStatus = 0xC000004F
	_STATUS_LOCK_NOT_GRANTED        ntStatus = 0xC0000055
	_STATUS_DELETE_PENDING          ntStatus = 0xC0000056
	_STATUS_DISK_FULL               ntStatus = 0xC000007F
	_STATUS_INSUFFICIENT_RESOURCES  ntStatus = 0xC000009A
	_STATUS_MEDIA_WRITE_PROTECTED   ntStatus = 0xC00000A2
	_STATUS_FILE_IS_A_DIRECTORY     ntStatus = 0xC00000BA
	_STATUS_NOT_SUPPORTED           ntStatus = 0xC00000BB
	_STATUS_NOT_SAME_DEVICE         ntStatus = 0xC00000D4
	_STATUS_DIRECTORY_NOT_EMPTY     ntStatus = 0xC0000101
	_STATUS_NOT_A_DIRECTORY         ntStatus = 0xC0000103
	_STATUS_NAME_TOO_LONG           ntStatus = 0xC0000106
	_STATUS_TOO_MANY_OPENED_FILES   ntStatus = 0xC000011F
	_STATUS_CANCELLED               ntStatus = 0xC0000120
	_STATUS_CANNOT_DELETE           ntStatus = 0xC0000121
	_STATUS_IO_DEVICE_ERROR         ntStatus = 0xC0000185
	_STATUS_TOO_MANY_LINKS          ntStatus = 0xC0000265
	_STATUS_NOT_A_REPARSE_POINT     ntStatus = 0xC0000275
	_STATUS_IO_REPARSE_TAG_MISMATCH ntStatus = 0xC0000277
	_STATUS_IO_REPARSE_DATA_INVALID ntStatus = 0xC0000278
)

func (s ntStatus) Error() string {
	return fmt.Sprintf("NTSTATUS 0x%08x", uint32(s))
}

// statusFromErrno maps an errno to NTSTATUS. The mapping follows
// WinFsp's FUSE layer. On Windows, a file system may also return
// Windows error codes, eg. from the os package.
func statusFromErrno(errno syscall.Errno) ntStatus {
	switch errno {
	case 0:
		return _STATUS_SUCCESS
	case syscall.EPERM, syscall.EACCES:
		return _STATUS_ACCESS_DENIED
	case syscall.ENOENT:
		return _STATUS_OBJECT_NAME_NOT_FOUND
	case syscall.EIO:
		return _STATUS_IO_DEVICE_ERROR
	case syscall.EBADF:
		return _STATUS_INVALID_HANDLE
	case syscall.ENOMEM, syscall.EAGAIN:
		return _STATUS_INSUFFICIENT_RESOURCES
	case syscall.EBUSY:
		return _STATUS_DEVICE_BUSY
	case syscall.EEXIST:
		return _STATUS_OBJECT_NAME_COLLISION
	case syscall.EXDEV:
		return _STATUS_NOT_SAME_DEVICE
	case syscall.ENODEV:
		return _STATUS_NO_SUCH_DEVICE
	case syscall.ENOTDIR:
		return _STATUS_NOT_A_DIRECTORY
	case syscall.EISDIR:
		return _STATUS_FILE_IS_A_DIRECTORY
	case syscall.EINVAL, syscall.ERANGE:
		return _STATUS_INVALID_PARAMETER
	case syscall.EMFILE, syscall.ENFILE:
		return _STATUS_TOO_MANY_OPENED_FILES
	case syscall.EFBIG, syscall.ENOSPC:
		return _STATUS_DISK_FULL
	case syscall.EROFS:
		return _STATUS_MEDIA_WRITE_PROTECTED
	case syscall.EMLINK:
		return _STATUS_TOO_MANY_LINKS
	case syscall.ENAMETOOLONG:
		return _STATUS_NAME_TOO_LONG
	case syscall.ENOSYS, syscall.ENOTSUP:
		return _STATUS_INVALID_DEVICE_REQUEST
	case syscall.ENOTEMPTY:
		return _STATUS_DIRECTORY_NOT_EMPTY
	case syscall.EINTR:
		return _STATUS_CANCELLED
	}
	if st, ok := statusFromSystemError(errno); ok {
		return st
	}
	return _STATUS_IO_DEVICE_ERROR
}
