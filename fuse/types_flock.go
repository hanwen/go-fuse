// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"io"
	"syscall"
)

func (lk *FileLock) ToFlockT(flockT *syscall.Flock_t) {
	flockT.Start = int64(lk.Start)
	if lk.End == (1<<63)-1 {
		flockT.Len = 0
	} else {
		flockT.Len = int64(lk.End - lk.Start + 1)
	}
	flockT.Whence = int16(io.SeekStart)
	flockT.Type = int16(lk.Typ)
}

func (lk *FileLock) FromFlockT(flockT *syscall.Flock_t) {
	lk.Typ = uint32(flockT.Type)
	if flockT.Type != syscall.F_UNLCK {
		lk.Start = uint64(flockT.Start)
		if flockT.Len == 0 {
			lk.End = (1 << 63) - 1
		} else {
			lk.End = uint64(flockT.Start + flockT.Len - 1)
		}
	}
	lk.Pid = uint32(flockT.Pid)
}
