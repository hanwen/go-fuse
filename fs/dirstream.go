// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type dirArray struct {
	idx     int
	entries []fuse.DirEntry
}

func (a *dirArray) HasNext() bool {
	return len(a.entries[a.idx:]) > 0
}

func (a *dirArray) Next() (fuse.DirEntry, syscall.Errno) {
	e := a.entries[a.idx]
	a.idx++
	return e, 0
}

func (a *dirArray) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	idx := int(off)
	if idx < 0 || idx > len(a.entries) {
		return syscall.EINVAL
	}
	a.idx = idx
	return 0
}

func (a *dirArray) Close() {

}

// NewListDirStream wraps a slice of DirEntry as a DirStream.
func NewListDirStream(list []fuse.DirEntry) DirStream {
	return &dirArray{entries: list}
}

// implement FileReaddirenter/FileReleasedirer
type dirStreamAsFile struct {
	creator func(context.Context) (DirStream, syscall.Errno)
	ds      DirStream
}

func (d *dirStreamAsFile) Releasedir(ctx context.Context, releaseFlags uint32) {
	if d.ds != nil {
		d.ds.Close()
	}
}

func (d *dirStreamAsFile) Readdirent(ctx context.Context) (de *fuse.DirEntry, errno syscall.Errno) {
	if d.ds == nil {
		d.ds, errno = d.creator(ctx)
		if errno != 0 {
			return nil, errno
		}
	}
	if !d.ds.HasNext() {
		return nil, 0
	}

	e, errno := d.ds.Next()
	return &e, errno
}

func (d *dirStreamAsFile) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	if d.ds == nil {
		var errno syscall.Errno
		d.ds, errno = d.creator(ctx)
		if errno != 0 {
			return errno
		}
	}
	if sd, ok := d.ds.(FileSeekdirer); ok {
		return sd.Seekdir(ctx, off)
	}
	return syscall.ENOTSUP
}
