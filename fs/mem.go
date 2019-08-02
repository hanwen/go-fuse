// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// MemRegularFile is a filesystem node that holds a read-only data
// slice in memory.
type MemRegularFile struct {
	Inode
	Data []byte
	Attr fuse.Attr
}

var _ = (NodeOpener)((*MemRegularFile)(nil))
var _ = (NodeReader)((*MemRegularFile)(nil))
var _ = (NodeFlusher)((*MemRegularFile)(nil))

func (f *MemRegularFile) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if flags&(syscall.O_RDWR) != 0 || flags&syscall.O_WRONLY != 0 {
		return nil, 0, syscall.EPERM
	}

	return nil, fuse.FOPEN_KEEP_CACHE, OK
}

var _ = (NodeGetattrer)((*MemRegularFile)(nil))

func (f *MemRegularFile) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr = f.Attr
	out.Attr.Size = uint64(len(f.Data))
	return OK
}

func (f *MemRegularFile) Flush(ctx context.Context, fh FileHandle) syscall.Errno {
	return 0
}

func (f *MemRegularFile) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(f.Data) {
		end = len(f.Data)
	}
	return fuse.ReadResultData(f.Data[off:end]), OK
}

// MemSymlink is an inode holding a symlink in memory.
type MemSymlink struct {
	Inode
	Attr fuse.Attr
	Data []byte
}

var _ = (NodeReadlinker)((*MemSymlink)(nil))

func (l *MemSymlink) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	return l.Data, OK
}

var _ = (NodeGetattrer)((*MemSymlink)(nil))

func (l *MemSymlink) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr = l.Attr
	return OK
}
