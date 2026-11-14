// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Random odds and ends.

package fuse

import (
	"errors"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
)

// errnoString formats e for debug output.
func errnoString(e syscall.Errno) string {
	if e == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", uintptr(e), e)
}

// ToErrno extracts an errno number from Go error objects. If it
// fails, it logs an error and returns ENOTSUP.
func ToErrno(err error) syscall.Errno {
	if err == nil {
		return OK
	}

	switch err {
	case os.ErrPermission:
		return EPERM
	case os.ErrExist:
		return syscall.EEXIST
	case os.ErrNotExist:
		return ENOENT
	case os.ErrInvalid:
		return EINVAL
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	log.Println("can't convert error type:", err)
	return ENOTSUP
}

func CurrentOwner() *Owner {
	return &Owner{
		Uid: uint32(os.Getuid()),
		Gid: uint32(os.Getgid()),
	}
}

// UtimeToTimespec converts a "Time" pointer as passed to Utimens to a
// "Timespec" that can be passed to the utimensat syscall.
// A nil pointer is converted to the special UTIME_OMIT value.
//
// Deprecated: use unix.TimeToTimespec from the x/sys/unix package instead.
func UtimeToTimespec(t *time.Time) (ts syscall.Timespec) {
	if t == nil {
		ts.Nsec = _UTIME_OMIT
	} else {
		ts = syscall.NsecToTimespec(t.UnixNano())
		// Go bug https://github.com/golang/go/issues/12777
		if ts.Nsec < 0 {
			ts.Nsec = 0
		}
	}
	return ts
}
