//go:build darwin
// +build darwin

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"
	"time"
)

var _ = (NodeGetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

var _ = (NodeSetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	return syscall.ENOSYS
}

var _ = (NodeRemovexattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
	return syscall.ENOSYS
}

var _ = (NodeListxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
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

var _ = (NodeCopyFileRanger)((*LoopbackNode)(nil))

func (n *LoopbackNode) CopyFileRange(ctx context.Context, fhIn FileHandle,
	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
	len uint64, flags uint64) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func intDev(dev uint32) int {
	return int(dev)
}
