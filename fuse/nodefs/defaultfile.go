// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type defaultFile struct{}

// NewDefaultFile returns a File instance that returns ENOSYS for
// every operation.
func NewDefaultFile() File {
	return (*defaultFile)(nil)
}

func (f *defaultFile) SetInode(*Inode) {
}

func (f *defaultFile) InnerFile() File {
	return nil
}

func (f *defaultFile) String() string {
	return "defaultFile"
}

func (f *defaultFile) Read(buf []byte, off int64, ctx *fuse.Context) (fuse.ReadResult, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (f *defaultFile) Write(data []byte, off int64, ctx *fuse.Context) (uint32, fuse.Status) {
	return 0, fuse.ENOSYS
}

func (f *defaultFile) GetLk(owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock, ctx *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *defaultFile) SetLk(owner uint64, lk *fuse.FileLock, flags uint32, ctx *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *defaultFile) SetLkw(owner uint64, lk *fuse.FileLock, flags uint32, ctx *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *defaultFile) Flush(ctx *fuse.Context) fuse.Status {
	return fuse.OK
}

func (f *defaultFile) Release() {

}

func (f *defaultFile) GetAttr(_ *fuse.Attr, ctx *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *defaultFile) Fsync(flags int, ctx *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *defaultFile) Utimens(atime *time.Time, mtime *time.Time, ctx *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *defaultFile) Truncate(size uint64, ctx *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *defaultFile) Chown(uid uint32, gid uint32, ctx *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *defaultFile) Chmod(perms uint32, ctx *fuse.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *defaultFile) Allocate(off uint64, size uint64, mode uint32, ctx *fuse.Context) (code fuse.Status) {
	return fuse.ENOSYS
}
