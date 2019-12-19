// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"fmt"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type lockingFile struct {
	mu   *sync.Mutex
	file File
}

// NewLockingFile serializes operations an existing File.
func NewLockingFile(mu *sync.Mutex, f File) File {
	return &lockingFile{
		mu:   mu,
		file: f,
	}
}

func (f *lockingFile) SetInode(*Inode) {
}

func (f *lockingFile) InnerFile() File {
	return f.file
}

func (f *lockingFile) String() string {
	return fmt.Sprintf("lockingFile(%s)", f.file.String())
}

func (f *lockingFile) Read(buf []byte, off int64, ctx *fuse.Context) (fuse.ReadResult, fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Read(buf, off, ctx)
}

func (f *lockingFile) Write(data []byte, off int64, ctx *fuse.Context) (uint32, fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Write(data, off, ctx)
}

func (f *lockingFile) Flush(ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Flush(ctx)
}

func (f *lockingFile) GetLk(owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock, ctx *fuse.Context) (code fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.GetLk(owner, lk, flags, out, ctx)
}

func (f *lockingFile) SetLk(owner uint64, lk *fuse.FileLock, flags uint32, ctx *fuse.Context) (code fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.SetLk(owner, lk, flags, ctx)
}

func (f *lockingFile) SetLkw(owner uint64, lk *fuse.FileLock, flags uint32, ctx *fuse.Context) (code fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.SetLkw(owner, lk, flags, ctx)
}

func (f *lockingFile) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.file.Release()
}

func (f *lockingFile) GetAttr(a *fuse.Attr, ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.GetAttr(a, ctx)
}

func (f *lockingFile) Fsync(flags int, ctx *fuse.Context) (code fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Fsync(flags, ctx)
}

func (f *lockingFile) Utimens(atime *time.Time, mtime *time.Time, ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Utimens(atime, mtime, ctx)
}

func (f *lockingFile) Truncate(size uint64, ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Truncate(size, ctx)
}

func (f *lockingFile) Chown(uid uint32, gid uint32, ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Chown(uid, gid, ctx)
}

func (f *lockingFile) Chmod(perms uint32, ctx *fuse.Context) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Chmod(perms, ctx)
}

func (f *lockingFile) Allocate(off uint64, size uint64, mode uint32, ctx *fuse.Context) (code fuse.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Allocate(off, size, mode, ctx)
}
