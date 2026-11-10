// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import "syscall"

func doCopyFileRange(fdIn int, offIn int64, fdOut int, offOut int64,
	len int, flags int) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func intDev(dev uint32) int {
	return int(dev)
}

// mknod creates regular files too: fuse(4) creates files with
// FUSE_MKNOD, but mknod(2) on OpenBSD only makes device nodes and
// FIFOs.
func mknod(path string, mode, rdev uint32) error {
	if mode&syscall.S_IFMT != syscall.S_IFREG {
		return syscall.Mknod(path, mode, intDev(rdev))
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_CLOEXEC, mode&^syscall.S_IFMT)
	if err != nil {
		return err
	}
	return syscall.Close(fd)
}

func birthtime(st *syscall.Stat_t) syscall.Timespec {
	return st.X__st_birthtim
}
