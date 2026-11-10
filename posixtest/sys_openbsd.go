// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package posixtest

import (
	"errors"
	"syscall"
)

// OpenBSD has no O_DIRECT, SEEK_DATA/SEEK_HOLE, extended attributes
// or OFD locks.
const (
	seekData = -1
	seekHole = -1

	// The kernel handles POSIX locks itself, so locks from one
	// process never conflict.
	kernelLocks = true
)

func getxattr(path, attr string, dest []byte) (int, error) {
	return 0, syscall.ENOTSUP
}

func setxattr(path, attr string, data []byte, flags int) error {
	return syscall.ENOTSUP
}

func listxattr(path string, dest []byte) (int, error) {
	return 0, syscall.ENOTSUP
}

func removexattr(path, attr string) error {
	return syscall.ENOTSUP
}

func sysFcntlFlockGetOFDLock(fd uintptr, lk *syscall.Flock_t) error {
	return errors.ErrUnsupported
}
