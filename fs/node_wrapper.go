// Copyright 2025 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// NodeWrapper enables wrapping existing node implementations, without
// knowing their concrete type. It implements all the Node interfaces.
// If the wrapped type implements a specific interface, it delegates
// to that implementation.  Otherwise, it returns syscall.ENOTSUP as a
// default. This can be used to like so:
//
//	type MyNodeWrapper struct {
//	  NodeWrapper
//	}
//
//	func wrapNode(node InodeEmbedder) *MyNodeWrapper {
//	  return &MyNodeWrapper{NodeWrapper{node}}
//	}
//
// See ExampleLoopbackReuse for a practical example.
type NodeWrapper struct {
	InodeEmbedder
}

// Compile-time assertions to ensure Wrapper implements all NodeXxxx interfaces
var (
	_ NodeStatfser       = (*NodeWrapper)(nil)
	_ NodeAccesser       = (*NodeWrapper)(nil)
	_ NodeGetattrer      = (*NodeWrapper)(nil)
	_ NodeSetattrer      = (*NodeWrapper)(nil)
	_ NodeOnAdder        = (*NodeWrapper)(nil)
	_ NodeGetxattrer     = (*NodeWrapper)(nil)
	_ NodeSetxattrer     = (*NodeWrapper)(nil)
	_ NodeRemovexattrer  = (*NodeWrapper)(nil)
	_ NodeListxattrer    = (*NodeWrapper)(nil)
	_ NodeReadlinker     = (*NodeWrapper)(nil)
	_ NodeOpener         = (*NodeWrapper)(nil)
	_ NodeReader         = (*NodeWrapper)(nil)
	_ NodeWriter         = (*NodeWrapper)(nil)
	_ NodeFsyncer        = (*NodeWrapper)(nil)
	_ NodeFlusher        = (*NodeWrapper)(nil)
	_ NodeReleaser       = (*NodeWrapper)(nil)
	_ NodeAllocater      = (*NodeWrapper)(nil)
	_ NodeCopyFileRanger = (*NodeWrapper)(nil)
	_ NodeStatxer        = (*NodeWrapper)(nil)
	_ NodeLseeker        = (*NodeWrapper)(nil)
	_ NodeGetlker        = (*NodeWrapper)(nil)
	_ NodeSetlker        = (*NodeWrapper)(nil)
	_ NodeSetlkwer       = (*NodeWrapper)(nil)
	_ NodeIoctler        = (*NodeWrapper)(nil)
	_ NodeOnForgetter    = (*NodeWrapper)(nil)
	_ NodeLookuper       = (*NodeWrapper)(nil)
	_ NodeWrapChilder    = (*NodeWrapper)(nil)
	_ NodeOpendirer      = (*NodeWrapper)(nil)
	_ NodeReaddirer      = (*NodeWrapper)(nil)
	_ NodeMkdirer        = (*NodeWrapper)(nil)
	_ NodeMknoder        = (*NodeWrapper)(nil)
	_ NodeLinker         = (*NodeWrapper)(nil)
	_ NodeSymlinker      = (*NodeWrapper)(nil)
	_ NodeCreater        = (*NodeWrapper)(nil)
	_ NodeUnlinker       = (*NodeWrapper)(nil)
	_ NodeRmdirer        = (*NodeWrapper)(nil)
	_ NodeRenamer        = (*NodeWrapper)(nil)
	_ NodeOpendirHandler = (*NodeWrapper)(nil)
)

// NodeStatfser implementation
func (w *NodeWrapper) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeStatfser); ok {
		return impl.Statfs(ctx, out)
	}
	return syscall.ENOTSUP
}

// NodeAccesser implementation
func (w *NodeWrapper) Access(ctx context.Context, mask uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeAccesser); ok {
		return impl.Access(ctx, mask)
	}
	return syscall.ENOTSUP
}

// NodeGetattrer implementation
func (w *NodeWrapper) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeGetattrer); ok {
		return impl.Getattr(ctx, f, out)
	}
	if f != nil {
		if fileImpl, ok := f.(FileGetattrer); ok {
			return fileImpl.Getattr(ctx, out)
		}
	}
	return syscall.ENOTSUP
}

// NodeSetattrer implementation
func (w *NodeWrapper) Setattr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeSetattrer); ok {
		return impl.Setattr(ctx, f, in, out)
	}
	if f != nil {
		if fileImpl, ok := f.(FileSetattrer); ok {
			return fileImpl.Setattr(ctx, in, out)
		}
	}
	return syscall.ENOTSUP
}

// NodeOnAdder implementation
func (w *NodeWrapper) OnAdd(ctx context.Context) {
	if impl, ok := w.InodeEmbedder.(NodeOnAdder); ok {
		impl.OnAdd(ctx)
	}
}

// NodeGetxattrer implementation
func (w *NodeWrapper) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeGetxattrer); ok {
		return impl.Getxattr(ctx, attr, dest)
	}
	return 0, syscall.ENOTSUP
}

// NodeSetxattrer implementation
func (w *NodeWrapper) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeSetxattrer); ok {
		return impl.Setxattr(ctx, attr, data, flags)
	}
	return syscall.ENOTSUP
}

// NodeRemovexattrer implementation
func (w *NodeWrapper) Removexattr(ctx context.Context, attr string) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeRemovexattrer); ok {
		return impl.Removexattr(ctx, attr)
	}
	return syscall.ENOTSUP
}

// NodeListxattrer implementation
func (w *NodeWrapper) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeListxattrer); ok {
		return impl.Listxattr(ctx, dest)
	}
	return 0, syscall.ENOTSUP
}

// NodeReadlinker implementation
func (w *NodeWrapper) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeReadlinker); ok {
		return impl.Readlink(ctx)
	}
	return nil, syscall.ENOTSUP
}

// NodeOpener implementation
func (w *NodeWrapper) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeOpener); ok {
		return impl.Open(ctx, flags)
	}
	return nil, 0, syscall.ENOTSUP
}

// NodeReader implementation
func (w *NodeWrapper) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeReader); ok {
		return impl.Read(ctx, f, dest, off)
	}
	if f != nil {
		if fileImpl, ok := f.(FileReader); ok {
			return fileImpl.Read(ctx, dest, off)
		}
	}
	return nil, syscall.ENOTSUP
}

// NodeWriter implementation
func (w *NodeWrapper) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeWriter); ok {
		return impl.Write(ctx, f, data, off)
	}
	if f != nil {
		if fileImpl, ok := f.(FileWriter); ok {
			return fileImpl.Write(ctx, data, off)
		}
	}
	return 0, syscall.ENOTSUP
}

// NodeFsyncer implementation
func (w *NodeWrapper) Fsync(ctx context.Context, f FileHandle, flags uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeFsyncer); ok {
		return impl.Fsync(ctx, f, flags)
	}
	if f != nil {
		if fileImpl, ok := f.(FileFsyncer); ok {
			return fileImpl.Fsync(ctx, flags)
		}
	}
	return syscall.ENOTSUP
}

// NodeFlusher implementation
func (w *NodeWrapper) Flush(ctx context.Context, f FileHandle) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeFlusher); ok {
		return impl.Flush(ctx, f)
	}
	if f != nil {
		if fileImpl, ok := f.(FileFlusher); ok {
			return fileImpl.Flush(ctx)
		}
	}
	return syscall.ENOTSUP
}

// NodeReleaser implementation
func (w *NodeWrapper) Release(ctx context.Context, f FileHandle) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeReleaser); ok {
		return impl.Release(ctx, f)
	}
	if f != nil {
		if fileImpl, ok := f.(FileReleaser); ok {
			return fileImpl.Release(ctx)
		}
	}
	return syscall.ENOTSUP
}

// NodeAllocater implementation
func (w *NodeWrapper) Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeAllocater); ok {
		return impl.Allocate(ctx, f, off, size, mode)
	}
	if f != nil {
		if fileImpl, ok := f.(FileAllocater); ok {
			return fileImpl.Allocate(ctx, off, size, mode)
		}
	}
	return syscall.ENOTSUP
}

// NodeCopyFileRanger implementation
func (w *NodeWrapper) CopyFileRange(ctx context.Context, fhIn FileHandle, offIn uint64, out *Inode, fhOut FileHandle, offOut uint64, len uint64, flags uint64) (uint32, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeCopyFileRanger); ok {
		return impl.CopyFileRange(ctx, fhIn, offIn, out, fhOut, offOut, len, flags)
	}
	return 0, syscall.ENOTSUP
}

// NodeStatxer implementation
func (w *NodeWrapper) Statx(ctx context.Context, f FileHandle, flags uint32, mask uint32, out *fuse.StatxOut) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeStatxer); ok {
		return impl.Statx(ctx, f, flags, mask, out)
	}
	if f != nil {
		if fileImpl, ok := f.(FileStatxer); ok {
			return fileImpl.Statx(ctx, flags, mask, out)
		}
	}
	return syscall.ENOTSUP
}

// NodeLseeker implementation
func (w *NodeWrapper) Lseek(ctx context.Context, f FileHandle, Off uint64, whence uint32) (uint64, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeLseeker); ok {
		return impl.Lseek(ctx, f, Off, whence)
	}
	if f != nil {
		if fileImpl, ok := f.(FileLseeker); ok {
			return fileImpl.Lseek(ctx, Off, whence)
		}
	}
	return 0, syscall.ENOTSUP
}

// NodeGetlker implementation
func (w *NodeWrapper) Getlk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeGetlker); ok {
		return impl.Getlk(ctx, f, owner, lk, flags, out)
	}
	if f != nil {
		if fileImpl, ok := f.(FileGetlker); ok {
			return fileImpl.Getlk(ctx, owner, lk, flags, out)
		}
	}
	return syscall.ENOTSUP
}

// NodeSetlker implementation
func (w *NodeWrapper) Setlk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeSetlker); ok {
		return impl.Setlk(ctx, f, owner, lk, flags)
	}
	if f != nil {
		if fileImpl, ok := f.(FileSetlker); ok {
			return fileImpl.Setlk(ctx, owner, lk, flags)
		}
	}
	return syscall.ENOTSUP
}

// NodeSetlkwer implementation
func (w *NodeWrapper) Setlkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeSetlkwer); ok {
		return impl.Setlkw(ctx, f, owner, lk, flags)
	}
	if f != nil {
		if fileImpl, ok := f.(FileSetlkwer); ok {
			return fileImpl.Setlkw(ctx, owner, lk, flags)
		}
	}
	return syscall.ENOTSUP
}

// NodeIoctler implementation
func (w *NodeWrapper) Ioctl(ctx context.Context, f FileHandle, cmd uint32, arg uint64, input []byte, output []byte) (result int32, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeIoctler); ok {
		return impl.Ioctl(ctx, f, cmd, arg, input, output)
	}
	if f != nil {
		if fileImpl, ok := f.(FileIoctler); ok {
			return fileImpl.Ioctl(ctx, cmd, arg, input, output)
		}
	}
	return 0, syscall.ENOTSUP
}

// NodeOnForgetter implementation
func (w *NodeWrapper) OnForget() {
	if impl, ok := w.InodeEmbedder.(NodeOnForgetter); ok {
		impl.OnForget()
	}
}

// NodeLookuper implementation
func (w *NodeWrapper) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeLookuper); ok {
		return impl.Lookup(ctx, name, out)
	}
	return nil, syscall.ENOTSUP
}

// NodeWrapChilder implementation
func (w *NodeWrapper) WrapChild(ctx context.Context, ops InodeEmbedder) InodeEmbedder {
	if impl, ok := w.InodeEmbedder.(NodeWrapChilder); ok {
		return impl.WrapChild(ctx, ops)
	}
	return ops
}

// NodeOpendirer implementation
func (w *NodeWrapper) Opendir(ctx context.Context) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeOpendirer); ok {
		return impl.Opendir(ctx)
	}
	return syscall.ENOTSUP
}

// NodeReaddirer implementation
func (w *NodeWrapper) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeReaddirer); ok {
		return impl.Readdir(ctx)
	}
	return nil, syscall.ENOTSUP
}

// NodeMkdirer implementation
func (w *NodeWrapper) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeMkdirer); ok {
		return impl.Mkdir(ctx, name, mode, out)
	}
	return nil, syscall.ENOTSUP
}

// NodeMknoder implementation
func (w *NodeWrapper) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeMknoder); ok {
		return impl.Mknod(ctx, name, mode, dev, out)
	}
	return nil, syscall.ENOTSUP
}

// NodeLinker implementation
func (w *NodeWrapper) Link(ctx context.Context, target InodeEmbedder, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeLinker); ok {
		return impl.Link(ctx, target, name, out)
	}
	return nil, syscall.ENOTSUP
}

// NodeSymlinker implementation
func (w *NodeWrapper) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeSymlinker); ok {
		return impl.Symlink(ctx, target, name, out)
	}
	return nil, syscall.ENOTSUP
}

// NodeCreater implementation
func (w *NodeWrapper) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeCreater); ok {
		return impl.Create(ctx, name, flags, mode, out)
	}
	return nil, nil, 0, syscall.ENOTSUP
}

// NodeUnlinker implementation
func (w *NodeWrapper) Unlink(ctx context.Context, name string) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeUnlinker); ok {
		return impl.Unlink(ctx, name)
	}
	return syscall.ENOTSUP
}

// NodeRmdirer implementation
func (w *NodeWrapper) Rmdir(ctx context.Context, name string) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeRmdirer); ok {
		return impl.Rmdir(ctx, name)
	}
	return syscall.ENOTSUP
}

// NodeRenamer implementation
func (w *NodeWrapper) Rename(ctx context.Context, name string, newParent InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if impl, ok := w.InodeEmbedder.(NodeRenamer); ok {
		return impl.Rename(ctx, name, newParent, newName, flags)
	}
	return syscall.ENOTSUP
}

// NodeOpendirHandler implementation
func (w *NodeWrapper) OpendirHandle(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	if impl, ok := w.InodeEmbedder.(NodeOpendirHandler); ok {
		return impl.OpendirHandle(ctx, flags)
	}
	return nil, 0, syscall.ENOTSUP
}
