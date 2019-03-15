// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"log"
	"sync/atomic"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

// DefaultOperations provides stubs that return ENOSYS for all functions
//
// It must be embedded in any Node implementation.
type DefaultOperations struct {
	inode_ *Inode
}

// check that we have implemented all interface methods
var _ Operations = &DefaultOperations{}

// set/retrieve inode.
//
// node -> inode association, can be simultaneously tried to be set, if for e.g.
//
//         root
//         /  \
//       dir1 dir2
//         \  /
//         file
//
// dir1.Lookup("file") and dir2.Lookup("file") are executed simultaneously.
//
// If not using FileID, the mapping in rawBridge does not help. So,
// use atomics so that only one set can win.
//
// To read node.inode atomic.LoadPointer is used, however it is not expensive
// since it translates to regular MOVQ on amd64.
func (n *DefaultOperations) setInode(inode *Inode) bool {
	return atomic.CompareAndSwapPointer(
		(*unsafe.Pointer)(unsafe.Pointer(&n.inode_)),
		nil, unsafe.Pointer(inode))
}

func (n *DefaultOperations) inode() *Inode {
	return (*Inode)(atomic.LoadPointer(
		(*unsafe.Pointer)(unsafe.Pointer(&n.inode_))))
}

func (n *DefaultOperations) StatFs(ctx context.Context, out *fuse.StatfsOut) fuse.Status {
	// this should be defined on OSX, or the FS won't mount
	*out = fuse.StatfsOut{}
	return fuse.OK
}

func (n *DefaultOperations) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOSYS
}

func (n *DefaultOperations) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOSYS
}

func (n *DefaultOperations) Access(ctx context.Context, mask uint32) fuse.Status {
	return fuse.ENOSYS
}

// ****************************************************************

func (n *DefaultOperations) FSetAttr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		return f.SetAttr(ctx, in, out)
	}

	return fuse.ENOSYS
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

func (n *DefaultOperations) OpenDir(ctx context.Context) fuse.Status {
	return fuse.ENOSYS
}

func (n *DefaultOperations) ReadDir(ctx context.Context) (DirStream, fuse.Status) {
	return nil, fuse.ENOSYS
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

func (n *DefaultOperations) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, status fuse.Status) {
	log.Println("defsyml")
	return nil, fuse.ENOSYS
}

func (n *DefaultOperations) Readlink(ctx context.Context) (string, fuse.Status) {
	return "", fuse.ENOSYS
}

func (n *DefaultOperations) Fsync(ctx context.Context, f FileHandle, flags uint32) fuse.Status {
	if f != nil {
		return f.Fsync(ctx, flags)
	}
	return fuse.ENOSYS
}

func (n *DefaultOperations) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, status fuse.Status) {
	if f != nil {
		return f.Write(ctx, data, off)
	}

	return 0, fuse.ENOSYS
}

func (n *DefaultOperations) GetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (status fuse.Status) {
	if f != nil {
		return f.GetLk(ctx, owner, lk, flags, out)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) SetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	if f != nil {
		return f.SetLk(ctx, owner, lk, flags)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) SetLkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
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

func (n *DefaultOperations) Release(ctx context.Context, f FileHandle) fuse.Status {
	if f != nil {
		return f.Release(ctx)
	}
	return fuse.ENOSYS
}

func (n *DefaultOperations) Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) (status fuse.Status) {
	if f != nil {
		return f.Allocate(ctx, off, size, mode)
	}

	return fuse.ENOSYS
}

func (n *DefaultOperations) FGetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		f.GetAttr(ctx, out)
	}
	return fuse.ENOSYS
}

func (n *DefaultOperations) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, status fuse.Status) {
	return nil, 0, fuse.ENOSYS
}

func (n *DefaultOperations) Create(ctx context.Context, name string, flags uint32, mode uint32) (node *Inode, fh FileHandle, fuseFlags uint32, status fuse.Status) {
	return nil, nil, 0, fuse.ENOSYS
}
func (n *DefaultOperations) Link(ctx context.Context, target Operations, name string, out *fuse.EntryOut) (node *Inode, status fuse.Status) {
	return nil, fuse.ENOSYS
}

func (n *DefaultOperations) GetXAttr(ctx context.Context, attr string, dest []byte) (uint32, fuse.Status) {
	return 0, fuse.ENOATTR
}

func (n *DefaultOperations) SetXAttr(ctx context.Context, attr string, data []byte, flags uint32) fuse.Status {
	return fuse.ENOATTR
}

func (n *DefaultOperations) RemoveXAttr(ctx context.Context, attr string) fuse.Status {
	return fuse.ENOATTR
}

func (n *DefaultOperations) ListXAttr(ctx context.Context, dest []byte) (uint32, fuse.Status) {
	return 0, fuse.OK
}

type DefaultFile struct {
}

var _ = FileHandle((*DefaultFile)(nil))

func (f *DefaultFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	return nil, fuse.ENOSYS
}

func (f *DefaultFile) Write(ctx context.Context, data []byte, off int64) (written uint32, status fuse.Status) {
	return 0, fuse.ENOSYS
}

func (f *DefaultFile) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (status fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) Flush(ctx context.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Release(ctx context.Context) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOSYS
}

func (f *DefaultFile) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (status fuse.Status) {
	return fuse.ENOSYS
}

func (f *DefaultFile) Fsync(ctx context.Context, flags uint32) (status fuse.Status) {
	return fuse.ENOSYS
}
