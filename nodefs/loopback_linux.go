// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"syscall"

	"golang.org/x/sys/unix"
)

func (n *loopbackNode) GetXAttr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	sz, err := syscall.Getxattr(n.path(), attr, dest)
	return uint32(sz), ToErrno(err)
}

func (n *loopbackNode) SetXAttr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	err := syscall.Setxattr(n.path(), attr, data, int(flags))
	return ToErrno(err)
}

func (n *loopbackNode) RemoveXAttr(ctx context.Context, attr string) syscall.Errno {
	err := syscall.Removexattr(n.path(), attr)
	return ToErrno(err)
}

func (n *loopbackNode) ListXAttr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	sz, err := syscall.Listxattr(n.path(), dest)
	return uint32(sz), ToErrno(err)
}

func (n *loopbackNode) renameExchange(name string, newparent *loopbackNode, newName string) syscall.Errno {
	fd1, err := syscall.Open(n.path(), syscall.O_DIRECTORY, 0)
	if err != nil {
		return ToErrno(err)
	}
	defer syscall.Close(fd1)
	fd2, err := syscall.Open(newparent.path(), syscall.O_DIRECTORY, 0)
	defer syscall.Close(fd2)
	if err != nil {
		return ToErrno(err)
	}

	var st syscall.Stat_t
	if err := syscall.Fstat(fd1, &st); err != nil {
		return ToErrno(err)
	}
	if !n.Inode().IsRoot() && n.Inode().NodeAttr().Ino != n.rootNode.idFromStat(&st).Ino {
		return syscall.EBUSY
	}
	if err := syscall.Fstat(fd2, &st); err != nil {
		return ToErrno(err)
	}
	if !newparent.Inode().IsRoot() && newparent.Inode().NodeAttr().Ino != n.rootNode.idFromStat(&st).Ino {
		return syscall.EBUSY
	}

	return ToErrno(unix.Renameat2(fd1, name, fd2, newName, unix.RENAME_EXCHANGE))
}

func (n *loopbackNode) CopyFileRange(ctx context.Context, fhIn FileHandle,
	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
	len uint64, flags uint64) (uint32, syscall.Errno) {
	lfIn, ok := fhIn.(*loopbackFile)
	if !ok {
		return 0, syscall.ENOTSUP
	}
	lfOut, ok := fhOut.(*loopbackFile)
	if !ok {
		return 0, syscall.ENOTSUP
	}

	signedOffIn := int64(offIn)
	signedOffOut := int64(offOut)
	count, err := unix.CopyFileRange(lfIn.fd, &signedOffIn, lfOut.fd, &signedOffOut, int(len), int(flags))
	return uint32(count), ToErrno(err)
}
