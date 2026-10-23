//go:build darwin
// +build darwin

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const unix_UTIME_OMIT = 0x0

const (
	dirOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY
	nodeOpenFlag = unix.O_SYMLINK
)

func aclKillSgid(ctx context.Context) bool { return false }

func setxattrKillSgid(dirfd int, name, attr string, data []byte, flags int) syscall.Errno {
	return syscall.ENOTSUP
}

// timeToTimeval - Convert time.Time to syscall.Timeval
//
// Note: This does not use syscall.NsecToTimespec because
// that does not work properly for times before 1970,
// see https://github.com/golang/go/issues/12777
func timeToTimeval(t *time.Time) syscall.Timeval {
	var tv syscall.Timeval
	tv.Usec = int32(t.Nanosecond() / 1000)
	tv.Sec = t.Unix()
	return tv
}

func doCopyFileRange(fdIn int, offIn int64, fdOut int, offOut int64,
	len int, flags int) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func intDev(dev uint32) int {
	return int(dev)
}

func statAt(dirfd int, name string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := unix.Fstatat(dirfd, name, (*unix.Stat_t)(unsafe.Pointer(st)), unix.AT_SYMLINK_NOFOLLOW)
	*btime = st.Birthtimespec
	return err
}

func fchmodatNoFollow(dirfd int, name string, mode uint32) error {
	return unix.Fchmodat(dirfd, name, mode, unix.AT_SYMLINK_NOFOLLOW)
}

func fstatFd(fd int, st *syscall.Stat_t, btime *syscall.Timespec) error {
	err := syscall.Fstat(fd, st)
	*btime = st.Birthtimespec
	return err
}

// path returns the absolute path to the node
func (n *LoopbackNode) path() string {
	return filepath.Join(n.RootData.Path, n.relativePath())
}

// Darwin has no mknodat.
func (n *LoopbackNode) mknodat(dirfd int, name string, mode, rdev uint32) error {
	return unix.Mknod(filepath.Join(n.path(), name), mode, intDev(rdev))
}
