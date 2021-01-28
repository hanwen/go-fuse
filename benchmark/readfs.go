// Copyright 2021 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

const fileSize = 2 << 60

// readFS is a filesystem that always and immediately returns zeros on read
// operations. Useful when benchmarking the raw throughput with go-fuse.
type readFS struct {
	fs.Inode
}

var _ = (fs.NodeLookuper)((*readFS)(nil))

func (n *readFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	sattr := fs.StableAttr{Mode: fuse.S_IFREG}
	return n.NewInode(ctx, &readFS{}, sattr), fs.OK
}

var _ = (fs.NodeGetattrer)((*readFS)(nil))

func (n *readFS) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = fileSize
	out.SetTimeout(time.Hour)
	return fs.OK
}

var _ = (fs.NodeOpener)((*readFS)(nil))

func (n *readFS) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return &readFS{}, fuse.FOPEN_DIRECT_IO, fs.OK
}

var _ = (fs.FileReader)((*readFS)(nil))

func (n *readFS) Read(ctx context.Context, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	return fuse.ReadResultData(dest), fs.OK
}
