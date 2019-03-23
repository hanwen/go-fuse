// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal"
)

// DefaultOperations provides no-operation default implementations for
// all the XxxOperations interfaces. The stubs provide useful defaults
// for implementing a read-only filesystem whose tree is constructed
// beforehand in the OnAdd method of the root. A example is in
// zip_test.go
//
// It must be embedded in any Operations implementation.
type DefaultOperations struct {
	inode_ Inode
}

// check that we have implemented all interface methods
var _ Operations = &DefaultOperations{}

func (n *DefaultOperations) inode() *Inode {
	return &n.inode_
}

func (n *DefaultOperations) init(ops Operations, attr NodeAttr, bridge *rawBridge, persistent bool) {
	n.inode_ = Inode{
		ops:        ops,
		nodeAttr:   attr,
		bridge:     bridge,
		persistent: persistent,
		parents:    make(map[parentData]struct{}),
	}
	if attr.Mode == fuse.S_IFDIR {
		n.inode_.children = make(map[string]*Inode)
	}
}

// Inode returns the Inode for this Operations
func (n *DefaultOperations) Inode() *Inode {
	return &n.inode_
}

// StatFs zeroes the out argument and returns OK.  This is because OSX
// filesystems must define this, or the mount will not work.
func (n *DefaultOperations) StatFs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	// this should be defined on OSX, or the FS won't mount
	*out = fuse.StatfsOut{}
	return OK
}

// The default OnAdd does nothing.
func (n *DefaultOperations) OnAdd(ctx context.Context) {
}

// GetAttr zeroes out argument and returns OK.
func (n *DefaultOperations) GetAttr(ctx context.Context, out *fuse.AttrOut) syscall.Errno {
	*out = fuse.AttrOut{}
	return OK
}

func (n *DefaultOperations) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	return syscall.EROFS
}

// The Access default implementation checks traditional unix
// permissions of the GetAttr result agains the caller.
func (n *DefaultOperations) Access(ctx context.Context, mask uint32) syscall.Errno {
	caller, ok := fuse.FromContext(ctx)
	if !ok {
		return syscall.EINVAL
	}

	var out fuse.AttrOut
	if s := n.inode().Operations().GetAttr(ctx, &out); s != 0 {
		return s
	}

	if !internal.HasAccess(caller.Uid, caller.Gid, out.Uid, out.Gid, out.Mode, mask) {
		return syscall.EACCES
	}
	return OK
}

// FSetAttr delegates to the FileHandle's if f is not nil, or else to the
// Inode's SetAttr method.
func (n *DefaultOperations) FSetAttr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		return f.SetAttr(ctx, in, out)
	}

	return n.inode_.Operations().SetAttr(ctx, in, out)
}

// The Lookup method on the DefaultOperations type looks for an
// existing child with the given name, or returns ENOENT.
func (n *DefaultOperations) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	ch := n.inode().GetChild(name)
	if ch == nil {
		return nil, syscall.ENOENT
	}

	var a fuse.AttrOut
	errno := ch.Operations().GetAttr(ctx, &a)
	out.Attr = a.Attr
	return ch, errno
}

// Mkdir returns EROFS
func (n *DefaultOperations) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

// Mknod returns EROFS
func (n *DefaultOperations) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	return nil, syscall.EROFS
}

// Rmdir returns EROFS
func (n *DefaultOperations) Rmdir(ctx context.Context, name string) syscall.Errno {
	return syscall.EROFS
}

// Unlink returns EROFS
func (n *DefaultOperations) Unlink(ctx context.Context, name string) syscall.Errno {
	return syscall.EROFS
}

// The default OpenDir always succeeds
func (n *DefaultOperations) OpenDir(ctx context.Context) syscall.Errno {
	return OK
}

// The default ReadDir returns the list of children from the tree
func (n *DefaultOperations) ReadDir(ctx context.Context) (DirStream, syscall.Errno) {
	r := []fuse.DirEntry{}
	for k, ch := range n.inode().Children() {
		r = append(r, fuse.DirEntry{Mode: ch.Mode(),
			Name: k,
			Ino:  ch.NodeAttr().Ino})
	}
	return NewListDirStream(r), 0
}

// Rename returns EROFS
func (n *DefaultOperations) Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) syscall.Errno {
	return syscall.EROFS
}

// Read delegates to the FileHandle argument.
func (n *DefaultOperations) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if f != nil {
		return f.Read(ctx, dest, off)
	}
	return nil, syscall.ENOTSUP
}

// Symlink returns EROFS
func (n *DefaultOperations) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno) {
	return nil, syscall.EROFS
}

// Readlink return ENOTSUP
func (n *DefaultOperations) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	return nil, syscall.ENOTSUP
}

// Fsync delegates to the FileHandle
func (n *DefaultOperations) Fsync(ctx context.Context, f FileHandle, flags uint32) syscall.Errno {
	if f != nil {
		return f.Fsync(ctx, flags)
	}
	return syscall.ENOTSUP
}

// Write delegates to the FileHandle
func (n *DefaultOperations) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, errno syscall.Errno) {
	if f != nil {
		return f.Write(ctx, data, off)
	}

	return 0, syscall.EROFS
}

func (n *DefaultOperations) CopyFileRange(ctx context.Context, fhIn FileHandle,
	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
	len uint64, flags uint64) (uint32, syscall.Errno) {
	return 0, syscall.EROFS
}

func (n *DefaultOperations) Lseek(ctx context.Context, f FileHandle, off uint64, whence uint32) (uint64, syscall.Errno) {
	if f != nil {
		return f.Lseek(ctx, off, whence)
	}
	return 0, syscall.ENOTSUP
}

// GetLk delegates to the FileHandlef
func (n *DefaultOperations) GetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
	if f != nil {
		return f.GetLk(ctx, owner, lk, flags, out)
	}

	return syscall.ENOTSUP
}

// SetLk delegates to the FileHandle
func (n *DefaultOperations) SetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	if f != nil {
		return f.SetLk(ctx, owner, lk, flags)
	}

	return syscall.ENOTSUP
}

// SetLkw delegates to the FileHandle
func (n *DefaultOperations) SetLkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	if f != nil {
		return f.SetLkw(ctx, owner, lk, flags)
	}

	return syscall.ENOTSUP
}

// Flush delegates to the FileHandle
func (n *DefaultOperations) Flush(ctx context.Context, f FileHandle) syscall.Errno {
	if f != nil {
		return f.Flush(ctx)
	}

	return syscall.ENOTSUP
}

// Release delegates to the FileHandle
func (n *DefaultOperations) Release(ctx context.Context, f FileHandle) syscall.Errno {
	if f != nil {
		return f.Release(ctx)
	}
	return OK
}

// Allocate delegates to the FileHandle
func (n *DefaultOperations) Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) (errno syscall.Errno) {
	if f != nil {
		return f.Allocate(ctx, off, size, mode)
	}

	return syscall.ENOTSUP
}

// FGetAttr delegates to the FileHandle's if f is not nil, or else to the
// Inode's GetAttr method.
func (n *DefaultOperations) FGetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		f.GetAttr(ctx, out)
	}
	return n.inode_.ops.GetAttr(ctx, out)
}

// Open returns ENOTSUP
func (n *DefaultOperations) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, 0, syscall.ENOTSUP
}

// Create returns ENOTSUP
func (n *DefaultOperations) Create(ctx context.Context, name string, flags uint32, mode uint32) (node *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, nil, 0, syscall.EROFS
}

// Link returns ENOTSUP
func (n *DefaultOperations) Link(ctx context.Context, target Operations, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno) {
	return nil, syscall.EROFS
}

// The default GetXAttr returns ENOATTR
func (n *DefaultOperations) GetXAttr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, ENOATTR
}

// The default SetXAttr returns ENOATTR
func (n *DefaultOperations) SetXAttr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	return syscall.EROFS
}

// The default RemoveXAttr returns ENOATTR
func (n *DefaultOperations) RemoveXAttr(ctx context.Context, attr string) syscall.Errno {
	return ENOATTR
}

// The default RemoveXAttr returns an empty list
func (n *DefaultOperations) ListXAttr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, OK
}

// DefaultFileHandle satisfies the FileHandle interface, and provides
// stub methods that return ENOTSUP for all operations.
type DefaultFileHandle struct {
}

var _ = FileHandle((*DefaultFileHandle)(nil))

func (f *DefaultFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	return nil, syscall.ENOTSUP
}

func (f *DefaultFileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	return 0, syscall.ENOTSUP
}

func (f *DefaultFileHandle) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) Flush(ctx context.Context) syscall.Errno {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) Release(ctx context.Context) syscall.Errno {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) GetAttr(ctx context.Context, out *fuse.AttrOut) syscall.Errno {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (errno syscall.Errno) {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) Fsync(ctx context.Context, flags uint32) (errno syscall.Errno) {
	return syscall.ENOTSUP
}

func (f *DefaultFileHandle) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	return 0, syscall.ENOTSUP
}
