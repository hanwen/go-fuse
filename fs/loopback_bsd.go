// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd || openbsd

package fs

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const unix_UTIME_OMIT = unix.UTIME_OMIT

const dirOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY

func aclKillSgid(ctx context.Context) bool { return false }

func setxattrKillSgid(dirfd int, name, attr string, data []byte, flags int) syscall.Errno {
	return syscall.ENOTSUP
}

func statAt(dirfd int, name string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := unix.Fstatat(dirfd, name, (*unix.Stat_t)(unsafe.Pointer(st)), unix.AT_SYMLINK_NOFOLLOW)
	*btime = birthtime(st)
	return err
}

func fstatFd(fd int, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := syscall.Fstat(fd, st)
	*btime = birthtime(st)
	return err
}

func fchmodatNoFollow(dirfd int, name string, mode uint32) error {
	return unix.Fchmodat(dirfd, name, mode, unix.AT_SYMLINK_NOFOLLOW)
}
