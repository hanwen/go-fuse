// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// statusFromSystemError maps Windows error codes, which differ from
// the errno values that Go defines on Windows. ERROR_FILE_NOT_FOUND and
// ERROR_PATH_NOT_FOUND are syscall.ENOENT and ENOTDIR, so
// statusFromErrno handles them.
func statusFromSystemError(errno syscall.Errno) (ntStatus, bool) {
	switch errno {
	case windows.ERROR_DIRECTORY:
		return _STATUS_NOT_A_DIRECTORY, true
	case windows.ERROR_ACCESS_DENIED:
		return _STATUS_ACCESS_DENIED, true
	case windows.ERROR_INVALID_HANDLE:
		return _STATUS_INVALID_HANDLE, true
	case windows.ERROR_NOT_ENOUGH_MEMORY, windows.ERROR_OUTOFMEMORY:
		return _STATUS_INSUFFICIENT_RESOURCES, true
	case windows.ERROR_WRITE_PROTECT:
		return _STATUS_MEDIA_WRITE_PROTECTED, true
	case windows.ERROR_NOT_SAME_DEVICE:
		return _STATUS_NOT_SAME_DEVICE, true
	case windows.ERROR_SHARING_VIOLATION:
		return _STATUS_SHARING_VIOLATION, true
	case windows.ERROR_LOCK_VIOLATION:
		return _STATUS_LOCK_NOT_GRANTED, true
	case windows.ERROR_HANDLE_EOF:
		return _STATUS_END_OF_FILE, true
	case windows.ERROR_NOT_SUPPORTED:
		return _STATUS_NOT_SUPPORTED, true
	case windows.ERROR_FILE_EXISTS, windows.ERROR_ALREADY_EXISTS:
		return _STATUS_OBJECT_NAME_COLLISION, true
	case windows.ERROR_INVALID_PARAMETER:
		return _STATUS_INVALID_PARAMETER, true
	case windows.ERROR_INVALID_NAME:
		return _STATUS_OBJECT_NAME_INVALID, true
	case windows.ERROR_DISK_FULL, windows.ERROR_HANDLE_DISK_FULL:
		return _STATUS_DISK_FULL, true
	case windows.ERROR_DIR_NOT_EMPTY:
		return _STATUS_DIRECTORY_NOT_EMPTY, true
	case windows.ERROR_FILENAME_EXCED_RANGE:
		return _STATUS_NAME_TOO_LONG, true
	case windows.ERROR_TOO_MANY_OPEN_FILES:
		return _STATUS_TOO_MANY_OPENED_FILES, true
	case windows.ERROR_DELETE_PENDING:
		return _STATUS_DELETE_PENDING, true
	case windows.ERROR_OPERATION_ABORTED, windows.ERROR_CANCELLED:
		return _STATUS_CANCELLED, true
	}
	return 0, false
}
