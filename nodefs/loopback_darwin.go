// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"syscall"
	"time"
)

func (n *loopbackNode) GetXAttr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func (n *loopbackNode) SetXAttr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	return syscall.ENOSYS
}

func (n *loopbackNode) RemoveXAttr(ctx context.Context, attr string) syscall.Errno {
	return syscall.ENOSYS
}

func (n *loopbackNode) ListXAttr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func (n *loopbackNode) renameExchange(name string, newparent *loopbackNode, newName string) syscall.Errno {
	return syscall.ENOSYS
}

func (f *loopbackFile) utimens(a *time.Time, m *time.Time) syscall.Errno {
	return syscall.ENOSYS // XXX
}
