// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

func (f *loopbackFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) fuse.Status {
	f.mu.Lock()
	err := syscall.Fallocate(int(f.File.Fd()), mode, int64(off), int64(sz))
	f.mu.Unlock()
	if err != nil {
		return fuse.ToStatus(err)
	}
	return fuse.OK
}

// Utimens - file handle based version of loopbackFileSystem.Utimens()
func (f *loopbackFile) Utimens(ctx context.Context, a *time.Time, m *time.Time) fuse.Status {
	var ts [2]syscall.Timespec
	ts[0] = fuse.UtimeToTimespec(a)
	ts[1] = fuse.UtimeToTimespec(m)
	f.mu.Lock()
	err := futimens(int(f.File.Fd()), &ts)
	f.mu.Unlock()
	return fuse.ToStatus(err)
}
