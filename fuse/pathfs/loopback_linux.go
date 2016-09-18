// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"fmt"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

func (fs *loopbackFileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	data, err := listXAttr(fs.GetPath(name))

	return data, fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	err := sysRemovexattr(fs.GetPath(name), attr)
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) String() string {
	return fmt.Sprintf("LoopbackFs(%s)", fs.Root)
}

func (fs *loopbackFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	data := make([]byte, 1024)
	data, err := getXAttr(fs.GetPath(name), attr, data)

	return data, fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	err := syscall.Setxattr(fs.GetPath(name), attr, data, flags)
	return fuse.ToStatus(err)
}

const _UTIME_NOW = ((1 << 30) - 1)
const _UTIME_OMIT = ((1 << 30) - 2)

// utimeToTimespec converts a "time.Time" pointer as passed to Utimens to a
// Timespec that can be passed to the utimensat syscall.
//
// Note: pathfs and nodefs both have a copy of this function, so make sure
// to update both.
func utimeToTimespec(t *time.Time) (ts syscall.Timespec) {
	if t == nil {
		ts.Nsec = _UTIME_OMIT
	} else {
		ts = syscall.NsecToTimespec(t.UnixNano())
		// For dates before 1970, NsecToTimespec incorrectly returns negative
		// nanoseconds. Ticket: https://github.com/golang/go/issues/12777
		if ts.Nsec < 0 {
			ts.Nsec = 0
		}
	}
	return ts
}

// Utimens - path based version of loopbackFile.Utimens()
func (fs *loopbackFileSystem) Utimens(path string, a *time.Time, m *time.Time, context *fuse.Context) (code fuse.Status) {
	var ts [2]syscall.Timespec
	ts[0] = utimeToTimespec(a)
	ts[1] = utimeToTimespec(m)
	err := sysUtimensat(0, fs.GetPath(path), &ts, _AT_SYMLINK_NOFOLLOW)
	return fuse.ToStatus(err)
}
