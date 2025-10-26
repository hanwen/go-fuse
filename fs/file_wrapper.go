// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// FileWrapper wraps a FileHandle. It implements all file operations
// and delegates to the wrapped handle, if it supports the
// operation. If the wrapped handle does not support the operation, it
// returns ENOTSUP.  This can be used to wrap existing FileHandle
// implementations without knowing their exact type, like so:
//
//	type MyWrapper struct {
//	    fs.FileWrapper
//	}
//
//	func wrap(fh FileHandle) *MyWrapper {
//	    return &MyWrapper{fs.FileWrapper{fh}}
//	}
type FileWrapper struct {
	FileHandle
}

// Compile-time assertions to ensure FileWrapper implements all FileXxxx interfaces
var (
	_ FilePassthroughFder = (*FileWrapper)(nil)
	_ FileReleaser        = (*FileWrapper)(nil)
	_ FileGetattrer       = (*FileWrapper)(nil)
	_ FileStatxer         = (*FileWrapper)(nil)
	_ FileReader          = (*FileWrapper)(nil)
	_ FileWriter          = (*FileWrapper)(nil)
	_ FileGetlker         = (*FileWrapper)(nil)
	_ FileSetlker         = (*FileWrapper)(nil)
	_ FileSetlkwer        = (*FileWrapper)(nil)
	_ FileLseeker         = (*FileWrapper)(nil)
	_ FileFlusher         = (*FileWrapper)(nil)
	_ FileFsyncer         = (*FileWrapper)(nil)
	_ FileSetattrer       = (*FileWrapper)(nil)
	_ FileAllocater       = (*FileWrapper)(nil)
	_ FileIoctler         = (*FileWrapper)(nil)
	_ FileReaddirenter    = (*FileWrapper)(nil)
	_ FileLookuper        = (*FileWrapper)(nil)
	_ FileFsyncdirer      = (*FileWrapper)(nil)
	_ FileSeekdirer       = (*FileWrapper)(nil)
	_ FileReleasedirer    = (*FileWrapper)(nil)
)

// NewFileWrapper creates a new FileWrapper instance with the given wrapped object.
func NewFileWrapper(wrapped interface{}) *FileWrapper {
	return &FileWrapper{
		FileHandle: wrapped,
	}
}

// FilePassthroughFder implementation
func (fw *FileWrapper) PassthroughFd() (int, bool) {
	if impl, ok := fw.FileHandle.(FilePassthroughFder); ok {
		return impl.PassthroughFd()
	}
	return 0, false
}

// FileReleaser implementation
func (fw *FileWrapper) Release(ctx context.Context) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileReleaser); ok {
		return impl.Release(ctx)
	}
	return syscall.ENOTSUP
}

// FileGetattrer implementation
func (fw *FileWrapper) Getattr(ctx context.Context, out *fuse.AttrOut) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileGetattrer); ok {
		return impl.Getattr(ctx, out)
	}
	return syscall.ENOTSUP
}

// FileStatxer implementation
func (fw *FileWrapper) Statx(ctx context.Context, flags uint32, mask uint32, out *fuse.StatxOut) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileStatxer); ok {
		return impl.Statx(ctx, flags, mask, out)
	}
	return syscall.ENOTSUP
}

// FileReader implementation
func (fw *FileWrapper) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileReader); ok {
		return impl.Read(ctx, dest, off)
	}
	return nil, syscall.ENOTSUP
}

// FileWriter implementation
func (fw *FileWrapper) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileWriter); ok {
		return impl.Write(ctx, data, off)
	}
	return 0, syscall.ENOTSUP
}

// FileGetlker implementation
func (fw *FileWrapper) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileGetlker); ok {
		return impl.Getlk(ctx, owner, lk, flags, out)
	}
	return syscall.ENOTSUP
}

// FileSetlker implementation
func (fw *FileWrapper) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileSetlker); ok {
		return impl.Setlk(ctx, owner, lk, flags)
	}
	return syscall.ENOTSUP
}

// FileSetlkwer implementation
func (fw *FileWrapper) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileSetlkwer); ok {
		return impl.Setlkw(ctx, owner, lk, flags)
	}
	return syscall.ENOTSUP
}

// FileLseeker implementation
func (fw *FileWrapper) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileLseeker); ok {
		return impl.Lseek(ctx, off, whence)
	}
	return 0, syscall.ENOTSUP
}

// FileFlusher implementation
func (fw *FileWrapper) Flush(ctx context.Context) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileFlusher); ok {
		return impl.Flush(ctx)
	}
	return syscall.ENOTSUP
}

// FileFsyncer implementation
func (fw *FileWrapper) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileFsyncer); ok {
		return impl.Fsync(ctx, flags)
	}
	return syscall.ENOTSUP
}

// FileSetattrer implementation
func (fw *FileWrapper) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileSetattrer); ok {
		return impl.Setattr(ctx, in, out)
	}
	return syscall.ENOTSUP
}

// FileAllocater implementation
func (fw *FileWrapper) Allocate(ctx context.Context, off uint64, size uint64, mode uint32) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileAllocater); ok {
		return impl.Allocate(ctx, off, size, mode)
	}
	return syscall.ENOTSUP
}

// FileIoctler implementation
func (fw *FileWrapper) Ioctl(ctx context.Context, cmd uint32, arg uint64, input []byte, output []byte) (result int32, errno syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileIoctler); ok {
		return impl.Ioctl(ctx, cmd, arg, input, output)
	}
	return 0, syscall.ENOTSUP
}

// FileReaddirenter implementation
func (fw *FileWrapper) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileReaddirenter); ok {
		return impl.Readdirent(ctx)
	}
	return nil, syscall.ENOTSUP
}

// FileLookuper implementation
func (fw *FileWrapper) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (child *Inode, errno syscall.Errno) {
	if impl, ok := fw.FileHandle.(FileLookuper); ok {
		return impl.Lookup(ctx, name, out)
	}
	return nil, syscall.ENOTSUP
}

// FileFsyncdirer implementation
func (fw *FileWrapper) Fsyncdir(ctx context.Context, flags uint32) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileFsyncdirer); ok {
		return impl.Fsyncdir(ctx, flags)
	}
	return syscall.ENOTSUP
}

// FileSeekdirer implementation
func (fw *FileWrapper) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	if impl, ok := fw.FileHandle.(FileSeekdirer); ok {
		return impl.Seekdir(ctx, off)
	}
	return syscall.ENOTSUP
}

// FileReleasedirer implementation
func (fw *FileWrapper) Releasedir(ctx context.Context, releaseFlags uint32) {
	if impl, ok := fw.FileHandle.(FileReleasedirer); ok {
		impl.Releasedir(ctx, releaseFlags)
	}
}
