// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd

package fs

import (
	"context"
	"syscall"

	"golang.org/x/sys/unix"
)

var _ = (NodeGetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	sz, err := unix.Lgetxattr(n.path(), attr, dest)
	return uint32(sz), ToErrno(err)
}

var _ = (NodeSetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	if aclKillSgid(ctx) {
		return setxattrKillSgid(n.RootData.Path, n.relativePath(), attr, data, int(flags))
	}
	err := unix.Lsetxattr(n.path(), attr, data, int(flags))
	return ToErrno(err)
}

var _ = (NodeRemovexattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
	err := unix.Lremovexattr(n.path(), attr)
	return ToErrno(err)
}
