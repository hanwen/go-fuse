// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// The syscall package has no futimens(2) on OpenBSD, and raw system
// calls are not allowed, so use futimes(2), filling in omitted times
// from Getattr.
func (f *LoopbackFile) utimens(a *time.Time, m *time.Time) syscall.Errno {
	var attr fuse.AttrOut
	if a == nil || m == nil {
		errno := f.Getattr(context.Background(), &attr)
		if errno != 0 {
			return errno
		}
	}
	if a == nil {
		t := time.Unix(int64(attr.Atime), int64(attr.Atimensec))
		a = &t
	}
	if m == nil {
		t := time.Unix(int64(attr.Mtime), int64(attr.Mtimensec))
		m = &t
	}
	tv := []syscall.Timeval{
		syscall.NsecToTimeval(a.UnixNano()),
		syscall.NsecToTimeval(m.UnixNano()),
	}
	return f.withFd(func(fd int) syscall.Errno {
		return ToErrno(syscall.Futimes(fd, tv))
	})
}
