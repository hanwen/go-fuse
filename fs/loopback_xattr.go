// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd

package fs

import (
	"context"
	"syscall"
)

var _ = (NodeGetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return 0, errno
	}
	defer syscall.Close(dirfd)
	sz, err := getxattrAt(dirfd, name, attr, dest)
	return uint32(sz), ToErrno(err)
}

var _ = (NodeSetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)
	if aclKillSgid(ctx) {
		return setxattrKillSgid(dirfd, name, attr, data, int(flags))
	}
	return ToErrno(setxattrAt(dirfd, name, attr, data, int(flags)))
}

var _ = (NodeRemovexattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)
	return ToErrno(removexattrAt(dirfd, name, attr))
}
