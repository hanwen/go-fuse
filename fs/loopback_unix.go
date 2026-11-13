//go:build !freebsd && !openbsd && !windows

// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package fs

import (
	"context"
	"syscall"
)

var _ = (NodeListxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return 0, errno
	}
	defer syscall.Close(dirfd)
	sz, err := listxattrAt(dirfd, name, dest)
	return uint32(sz), ToErrno(err)
}
