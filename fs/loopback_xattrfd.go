// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd

package fs

import (
	"golang.org/x/sys/unix"
)

func openNode(dirfd int, name string) (int, error) {
	return unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|nodeOpenFlag, 0)
}

func getxattrAt(dirfd int, name, attr string, dest []byte) (int, error) {
	fd, err := openNode(dirfd, name)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	return unix.Fgetxattr(fd, attr, dest)
}

func setxattrAt(dirfd int, name, attr string, data []byte, flags int) error {
	fd, err := openNode(dirfd, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsetxattr(fd, attr, data, flags)
}

func removexattrAt(dirfd int, name, attr string) error {
	fd, err := openNode(dirfd, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fremovexattr(fd, attr)
}

func listxattrAt(dirfd int, name string, dest []byte) (int, error) {
	fd, err := openNode(dirfd, name)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	return unix.Flistxattr(fd, dest)
}
