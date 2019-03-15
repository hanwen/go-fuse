// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"sync/atomic"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

// DefaultOperations provides no-operation default implementations for
// all the XxxOperations interfaces. The stubs provide useful defaults
// for implementing a read-only filesystem whose tree is constructed
// beforehand in the OnAdd method of the root. A example is in
// zip_test.go
//
// It must be embedded in any Operations implementation.
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

// StatFs zeroes the out argument and returns OK.  This is because OSX
// filesystems must define this, or the mount will not work.
func (n *DefaultOperations) StatFs(ctx context.Context, out *fuse.StatfsOut) fuse.Status {
	// this should be defined on OSX, or the FS won't mount
	*out = fuse.StatfsOut{}
	return fuse.OK
}

func (n *DefaultOperations) OnAdd() {
	// XXX context?
}

// GetAttr zeroes out argument and returns OK.
func (n *DefaultOperations) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	*out = fuse.AttrOut{}
	return fuse.OK
}

func (n *DefaultOperations) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	return fuse.EROFS
}

func (n *DefaultOperations) Access(ctx context.Context, mask uint32) fuse.Status {
	return fuse.ENOENT
}

// FSetAttr delegates to the FileHandle's if f is not nil, or else to the
// Inode's SetAttr method.
func (n *DefaultOperations) FSetAttr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		return f.SetAttr(ctx, in, out)
	}

	return n.inode_.Operations().SetAttr(ctx, in, out)
}

// The Lookup method on the DefaultOperations type looks for an
// existing child with the given name, or returns ENOENT.
func (n *DefaultOperations) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	ch := InodeOf(n).GetChild(name)
	if ch == nil {
		return nil, fuse.ENOENT
	}

	var a fuse.AttrOut
	status := ch.Operations().GetAttr(ctx, &a)
	out.Attr = a.Attr
	return ch, status
}

// Mkdir returns EROFS
func (n *DefaultOperations) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	return nil, fuse.EROFS
}

// Mknod returns EROFS
func (n *DefaultOperations) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	return nil, fuse.EROFS
}

// Rmdir returns EROFS
func (n *DefaultOperations) Rmdir(ctx context.Context, name string) fuse.Status {
	return fuse.EROFS
}

// Unlink returns EROFS
func (n *DefaultOperations) Unlink(ctx context.Context, name string) fuse.Status {
	return fuse.EROFS
}

// The default OpenDir always succeeds
func (n *DefaultOperations) OpenDir(ctx context.Context) fuse.Status {
	return fuse.OK
}

// The default ReadDir returns the list of children from the tree
func (n *DefaultOperations) ReadDir(ctx context.Context) (DirStream, fuse.Status) {
	r := []fuse.DirEntry{}
	for k, ch := range InodeOf(n).Children() {
		r = append(r, fuse.DirEntry{Mode: ch.Mode(),
			Name: k,
			Ino:  ch.FileID().Ino})
	}
	return NewListDirStream(r), fuse.OK
}

// Rename returns EROFS
func (n *DefaultOperations) Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) fuse.Status {
	return fuse.EROFS
}

func (n *DefaultOperations) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	if f != nil {
		return f.Read(ctx, dest, off)
	}
	return nil, fuse.ENOENT
}

// Symlink returns EROFS
func (n *DefaultOperations) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, status fuse.Status) {
	return nil, fuse.EROFS
}

func (n *DefaultOperations) Readlink(ctx context.Context) (string, fuse.Status) {
	return "", fuse.ENOENT
}

func (n *DefaultOperations) Fsync(ctx context.Context, f FileHandle, flags uint32) fuse.Status {
	if f != nil {
		return f.Fsync(ctx, flags)
	}
	return fuse.ENOENT
}

func (n *DefaultOperations) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, status fuse.Status) {
	if f != nil {
		return f.Write(ctx, data, off)
	}

	return 0, fuse.EROFS
}

func (n *DefaultOperations) GetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (status fuse.Status) {
	if f != nil {
		return f.GetLk(ctx, owner, lk, flags, out)
	}

	return fuse.ENOENT
}

func (n *DefaultOperations) SetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	if f != nil {
		return f.SetLk(ctx, owner, lk, flags)
	}

	return fuse.ENOENT
}

func (n *DefaultOperations) SetLkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	if f != nil {
		return f.SetLkw(ctx, owner, lk, flags)
	}

	return fuse.ENOENT
}
func (n *DefaultOperations) Flush(ctx context.Context, f FileHandle) fuse.Status {
	if f != nil {
		return f.Flush(ctx)
	}

	return fuse.ENOENT
}

func (n *DefaultOperations) Release(ctx context.Context, f FileHandle) fuse.Status {
	if f != nil {
		return f.Release(ctx)
	}
	return fuse.OK
}

func (n *DefaultOperations) Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) (status fuse.Status) {
	if f != nil {
		return f.Allocate(ctx, off, size, mode)
	}

	return fuse.ENOENT
}

// FGetAttr delegates to the FileHandle's if f is not nil, or else to the
// Inode's GetAttr method.
func (n *DefaultOperations) FGetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		f.GetAttr(ctx, out)
	}
	return n.inode_.ops.GetAttr(ctx, out)
}

func (n *DefaultOperations) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, status fuse.Status) {
	return nil, 0, fuse.ENOENT
}

func (n *DefaultOperations) Create(ctx context.Context, name string, flags uint32, mode uint32) (node *Inode, fh FileHandle, fuseFlags uint32, status fuse.Status) {
	return nil, nil, 0, fuse.EROFS
}

func (n *DefaultOperations) Link(ctx context.Context, target Operations, name string, out *fuse.EntryOut) (node *Inode, status fuse.Status) {
	return nil, fuse.EROFS
}

// The default GetXAttr returns ENOATTR
func (n *DefaultOperations) GetXAttr(ctx context.Context, attr string, dest []byte) (uint32, fuse.Status) {
	return 0, fuse.ENOATTR
}

// The default SetXAttr returns ENOATTR
func (n *DefaultOperations) SetXAttr(ctx context.Context, attr string, data []byte, flags uint32) fuse.Status {
	return fuse.EROFS
}

// The default RemoveXAttr returns ENOATTR
func (n *DefaultOperations) RemoveXAttr(ctx context.Context, attr string) fuse.Status {
	return fuse.ENOATTR
}

// The default RemoveXAttr returns an empty list
func (n *DefaultOperations) ListXAttr(ctx context.Context, dest []byte) (uint32, fuse.Status) {
	return 0, fuse.OK
}

// DefaultFileHandle satisfies the FileHandle interface, and provides
// stub methods that return ENOENT for all operations.
type DefaultFileHandle struct {
}

var _ = FileHandle((*DefaultFileHandle)(nil))

func (f *DefaultFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	return nil, fuse.ENOENT
}

func (f *DefaultFileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, status fuse.Status) {
	return 0, fuse.ENOENT
}

func (f *DefaultFileHandle) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (status fuse.Status) {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) Flush(ctx context.Context) fuse.Status {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) Release(ctx context.Context) fuse.Status {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (status fuse.Status) {
	return fuse.ENOENT
}

func (f *DefaultFileHandle) Fsync(ctx context.Context, flags uint32) (status fuse.Status) {
	return fuse.ENOENT
}
