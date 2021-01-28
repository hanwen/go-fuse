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

type ReadFS struct {
	fs.Inode
}

var _ = (fs.NodeLookuper)((*ReadFS)(nil))

func (n *ReadFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	sattr := fs.StableAttr{Mode: fuse.S_IFREG}
	return n.NewInode(ctx, &ReadFS{}, sattr), fs.OK
}

var _ = (fs.NodeGetattrer)((*ReadFS)(nil))

func (n *ReadFS) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = fileSize
	out.SetTimeout(time.Hour)
	return fs.OK
}

var _ = (fs.NodeOpener)((*ReadFS)(nil))

func (n *ReadFS) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return &ReadFS{}, fuse.FOPEN_DIRECT_IO, fs.OK
}

var _ = (fs.FileReader)((*ReadFS)(nil))

func (n *ReadFS) Read(ctx context.Context, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	return fuse.ReadResultData(dest), fs.OK
}
