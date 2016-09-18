// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

func (f *loopbackFile) Allocate(off uint64, sz uint64, mode uint32) fuse.Status {
	f.lock.Lock()
	err := syscall.Fallocate(int(f.File.Fd()), mode, int64(off), int64(sz))
	f.lock.Unlock()
	if err != nil {
		return fuse.ToStatus(err)
	}
	return fuse.OK
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

// Utimens - file handle based version of loopbackFileSystem.Utimens()
func (f *loopbackFile) Utimens(a *time.Time, m *time.Time) fuse.Status {
	var ts [2]syscall.Timespec
	ts[0] = utimeToTimespec(a)
	ts[1] = utimeToTimespec(m)
	f.lock.Lock()
	err := futimens(int(f.File.Fd()), &ts)
	f.lock.Unlock()
	return fuse.ToStatus(err)
}
