// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd || openbsd

package fs

import (
	"context"
	"syscall"

	"golang.org/x/sys/unix"
)

const unix_UTIME_OMIT = unix.UTIME_OMIT

func aclKillSgid(ctx context.Context) bool { return false }

func setxattrKillSgid(root, rel, attr string, data []byte, flags int) syscall.Errno {
	return syscall.ENOTSUP
}

func lstat(path string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := syscall.Lstat(path, st)
	*btime = birthtime(st)
	return err
}

func stat(path string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := syscall.Stat(path, st)
	*btime = birthtime(st)
	return err
}

func fstatFd(fd int, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := syscall.Fstat(fd, st)
	*btime = birthtime(st)
	return err
}
