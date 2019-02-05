// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

// DefaultNode provides common base Node functionality.
//
// It must be embedded in any Node implementation.
type DefaultNode struct {
	inode_ *Inode
}

// set/retrieve inode.
//
// node -> inode association, can be simultaneously tried to be set, if for e.g.
//
//	    root
//	    /  \
//	  dir1 dir2
//	    \  /
//	    file
//
// dir1.Lookup("file") and dir2.Lookup("file") are executed simultaneously.
//
// We use atomics so that only one set can win
//
// To read node.inode atomic.LoadPointer is used, however it is not expensive
// since it translates to regular MOVQ on amd64.

func (dn *DefaultNode) setInode(inode *Inode) bool {
	return atomic.CompareAndSwapPointer(
		(*unsafe.Pointer)(unsafe.Pointer(&dn.inode_)),
		nil, unsafe.Pointer(inode))
}

func (dn *DefaultNode) inode() *Inode {
	return (*Inode)(atomic.LoadPointer(
		(*unsafe.Pointer)(unsafe.Pointer(&dn.inode_))))
}

func (n *DefaultNode) Read(ctx context.Context, f File, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	if f != nil {
		return f.Read(ctx, dest, off)
	}
	return nil, fuse.ENOSYS
}
func (n *DefaultNode) Write(ctx context.Context, f File, data []byte, off int64) (written uint32, code fuse.Status) {
	if f != nil {
		return f.Write(ctx, data, off)
	}

	return 0, fuse.ENOSYS
}

func (n *DefaultNode) GetLk(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status) {
	if f != nil {
		return f.GetLk(ctx, owner, lk, flags, out)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) SetLk(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	if f != nil {
		return f.SetLk(ctx, owner, lk, flags)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) SetLkw(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	if f != nil {
		return f.SetLkw(ctx, owner, lk, flags)
	}

	return fuse.ENOSYS
}
func (n *DefaultNode) Flush(ctx context.Context, f File) fuse.Status {
	if f != nil {
		return f.Flush(ctx)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) Release(ctx context.Context, f File) {
	if f != nil {
		f.Release(ctx)
	}
}

func (n *DefaultNode) Allocate(ctx context.Context, f File, off uint64, size uint64, mode uint32) (code fuse.Status) {
	if f != nil {
		return f.Allocate(ctx, off, size, mode)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) GetAttr(ctx context.Context, f File, out *fuse.Attr) fuse.Status {
	if f != nil {
		f.GetAttr(ctx, out)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) Truncate(ctx context.Context, f File, size uint64) fuse.Status {
	if f != nil {
		return f.Truncate(ctx, size)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) Chown(ctx context.Context, f File, uid uint32, gid uint32) fuse.Status {
	if f != nil {
		return f.Chown(ctx, uid, gid)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) Chmod(ctx context.Context, f File, perms uint32) fuse.Status {
	if f != nil {
		return f.Chmod(ctx, perms)
	}

	return fuse.ENOSYS
}

func (n *DefaultNode) Utimens(ctx context.Context, f File, atime *time.Time, mtime *time.Time) fuse.Status {
	if f != nil {
		return f.Utimens(ctx, atime, mtime)
	}

	return fuse.ENOSYS
}

type DefaultFile struct {
}

func (f *DefaultFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (f *DefaultFile) Write(ctx context.Context, data []byte, off int64) (written uint32, code fuse.Status) {
	return 0, fuse.ENOSYS
}

func (f *DefaultFile) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) Flush(ctx context.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Release(ctx context.Context) {

}

func (f *DefaultFile) GetAttr(ctx context.Context, out *fuse.Attr) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Truncate(ctx context.Context, size uint64) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Chown(ctx context.Context, uid uint32, gid uint32) fuse.Status {
	return fuse.ENOSYS

}

func (f *DefaultFile) Chmod(ctx context.Context, perms uint32) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Utimens(ctx context.Context, atime *time.Time, mtime *time.Time) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (code fuse.Status) {
	return fuse.ENOSYS
}
