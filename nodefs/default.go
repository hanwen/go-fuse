// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// DefaultOperations provides common base Node functionality.
//
// It must be embedded in any Node implementation.
type DefaultOperations struct {
	inode_ *Inode
}

// check that we have implemented all interface methods
var _ Operations = &DefaultOperations{}

func (dn *DefaultOperations) setInode(inode *Inode) {
	dn.inode_ = inode
}

func (dn *DefaultOperations) inode() *Inode {
	return dn.inode_
}

func (n *DefaultOperations) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (n *DefaultOperations) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	return nil, fuse.ENOSYS
}
func (n *DefaultOperations) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	return nil, fuse.ENOSYS
}
func (n *DefaultOperations) Rmdir(ctx context.Context, name string) fuse.Status {
	return fuse.ENOSYS
}
func (n *DefaultOperations) Unlink(ctx context.Context, name string) fuse.Status {
	return fuse.ENOSYS
}

func (n *DefaultOperations) Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) fuse.Status {
	return fuse.ENOSYS
}

func (n *DefaultOperations) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	if f != nil {
		return f.Read(ctx, dest, off)
	}
	return nil, fuse.ENOSYS
}

func (n *DefaultOperations) Fsync(ctx context.Context, f FileHandle, flags uint32) fuse.Status {
	if f != nil {
		return f.Fsync(ctx, flags)
	}
	return fuse.ENOSYS
}

func (n *DefaultOperations) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, code fuse.Status) {
	if f != nil {
		return f.Write(ctx, data, off)
	}

	return 0, fuse.ENOSYS
}

func (n *DefaultOperations) GetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status) {
	if f != nil {
		return f.GetLk(ctx, owner, lk, flags, out)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) SetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	if f != nil {
		return f.SetLk(ctx, owner, lk, flags)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) SetLkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	if f != nil {
		return f.SetLkw(ctx, owner, lk, flags)
	}

	return fuse.ENOSYS
}
func (n *DefaultOperations) Flush(ctx context.Context, f FileHandle) fuse.Status {
	if f != nil {
		return f.Flush(ctx)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Release(ctx context.Context, f FileHandle) {
	if f != nil {
		f.Release(ctx)
	}
}

func (n *DefaultOperations) Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) (code fuse.Status) {
	if f != nil {
		return f.Allocate(ctx, off, size, mode)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) GetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		f.GetAttr(ctx, out)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Truncate(ctx context.Context, f FileHandle, size uint64) fuse.Status {
	if f != nil {
		return f.Truncate(ctx, size)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Chown(ctx context.Context, f FileHandle, uid uint32, gid uint32) fuse.Status {
	if f != nil {
		return f.Chown(ctx, uid, gid)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Chmod(ctx context.Context, f FileHandle, perms uint32) fuse.Status {
	if f != nil {
		return f.Chmod(ctx, perms)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Utimens(ctx context.Context, f FileHandle, atime *time.Time, mtime *time.Time) fuse.Status {
	if f != nil {
		return f.Utimens(ctx, atime, mtime)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, code fuse.Status) {
	return nil, 0, fuse.ENOSYS
}

func (n *DefaultOperations) Create(ctx context.Context, name string, flags uint32, mode uint32) (node *Inode, fh FileHandle, fuseFlags uint32, code fuse.Status) {
	return nil, nil, 0, fuse.ENOSYS
}

type DefaultFile struct {
}

var _ = FileHandle((*DefaultFile)(nil))

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

func (f *DefaultFile) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
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

func (f *DefaultFile) Fsync(ctx context.Context, flags uint32) (code fuse.Status) {
	return fuse.ENOSYS
}
