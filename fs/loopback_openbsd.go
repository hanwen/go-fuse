// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func doCopyFileRange(fdIn int, offIn int64, fdOut int, offOut int64,
	len int, flags int) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func intDev(dev uint32) int {
	return int(dev)
}

// mknodat creates regular files too: fuse(4) creates files with
// FUSE_MKNOD, but mknod(2) on OpenBSD only makes device nodes and
// FIFOs.
func (n *LoopbackNode) mknodat(dirfd int, name string, mode, rdev uint32) error {
	if mode&syscall.S_IFMT != syscall.S_IFREG {
		return unix.Mknodat(dirfd, name, mode, intDev(rdev))
	}
	fd, err := unix.Openat(dirfd, name, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode&^syscall.S_IFMT)
	if err != nil {
		return err
	}
	return unix.Close(fd)
}

func birthtime(st *syscall.Stat_t) syscall.Timespec {
	return st.X__st_birthtim
}
