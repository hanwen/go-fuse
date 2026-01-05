// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"sync"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/fallocate"
	"github.com/hanwen/go-fuse/v2/internal/ioctl"
	"golang.org/x/sys/unix"
)

// NewLoopbackFile creates a FileHandle out of a file descriptor. All
// operations are implemented. When using the Fd from a *os.File, call
// syscall.Dup() on the fd, to avoid os.File's finalizer from closing
// the file descriptor.
// It always returns a *LoopbackFile, but the return type is FileHandle for compatibility with other implementations.
func NewLoopbackFile(fd int) FileHandle {
	return &LoopbackFile{fd: fd}
}

// The LoopbackFile implements FileHandle by forwarding all calls to the underlying file descriptor.
// Create an instance by using NewLoopbackFile() above.
// If needed, cast the returned FileHandle to *LoopbackFile to extend its methods.
type LoopbackFile struct {
	mu sync.Mutex
	fd int
}

var _ = (FileHandle)((*LoopbackFile)(nil))
var _ = (FileReleaser)((*LoopbackFile)(nil))
var _ = (FileGetattrer)((*LoopbackFile)(nil))
var _ = (FileReader)((*LoopbackFile)(nil))
var _ = (FileWriter)((*LoopbackFile)(nil))
var _ = (FileGetlker)((*LoopbackFile)(nil))
var _ = (FileSetlker)((*LoopbackFile)(nil))
var _ = (FileSetlkwer)((*LoopbackFile)(nil))
var _ = (FileLseeker)((*LoopbackFile)(nil))
var _ = (FileFlusher)((*LoopbackFile)(nil))
var _ = (FileFsyncer)((*LoopbackFile)(nil))
var _ = (FileSetattrer)((*LoopbackFile)(nil))
var _ = (FileAllocater)((*LoopbackFile)(nil))
var _ = (FilePassthroughFder)((*LoopbackFile)(nil))
var _ = (FileIoctler)((*LoopbackFile)(nil))

func (f *LoopbackFile) PassthroughFd() (int, bool) {
	// This Fd is not accessed concurrently, but lock anyway for uniformity.
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fd, true
}

func (f *LoopbackFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := fuse.ReadResultFd(uintptr(f.fd), off, len(buf))
	return r, OK
}

func (f *LoopbackFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := syscall.Pwrite(f.fd, data, off)
	return uint32(n), ToErrno(err)
}

func (f *LoopbackFile) Release(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fd != -1 {
		err := syscall.Close(f.fd)
		f.fd = -1
		return ToErrno(err)
	}
	return syscall.EBADF
}

func (f *LoopbackFile) Flush(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Since Flush() may be called for each dup'd fd, we don't
	// want to really close the file, we just want to flush. This
	// is achieved by closing a dup'd fd.
	newFd, err := syscall.Dup(f.fd)

	if err != nil {
		return ToErrno(err)
	}
	err = syscall.Close(newFd)
	return ToErrno(err)
}

func (f *LoopbackFile) Fsync(ctx context.Context, flags uint32) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := ToErrno(syscall.Fsync(f.fd))

	return r
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

func (f *LoopbackFile) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	flk := syscall.Flock_t{}
	lk.ToFlockT(&flk)
	errno = ToErrno(syscall.FcntlFlock(uintptr(f.fd), _OFD_GETLK, &flk))
	out.FromFlockT(&flk)
	return
}

func (f *LoopbackFile) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return f.setLock(ctx, owner, lk, flags, false)
}

func (f *LoopbackFile) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return f.setLock(ctx, owner, lk, flags, true)
}

func (f *LoopbackFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if (flags & fuse.FUSE_LK_FLOCK) != 0 {
		var op int
		switch lk.Typ {
		case syscall.F_RDLCK:
			op = syscall.LOCK_SH
		case syscall.F_WRLCK:
			op = syscall.LOCK_EX
		case syscall.F_UNLCK:
			op = syscall.LOCK_UN
		default:
			return syscall.EINVAL
		}
		if !blocking {
			op |= syscall.LOCK_NB
		}
		return ToErrno(syscall.Flock(f.fd, op))
	} else {
		flk := syscall.Flock_t{}
		lk.ToFlockT(&flk)
		var op int
		if blocking {
			op = _OFD_SETLKW
		} else {
			op = _OFD_SETLK
		}
		return ToErrno(syscall.FcntlFlock(uintptr(f.fd), op, &flk))
	}
}

func (f *LoopbackFile) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if errno := f.setAttr(ctx, in); errno != 0 {
		return errno
	}

	return f.Getattr(ctx, out)
}

func (f *LoopbackFile) fchmod(mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	return ToErrno(syscall.Fchmod(f.fd, mode))
}

func (f *LoopbackFile) fchown(uid, gid int) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	return ToErrno(syscall.Fchown(f.fd, uid, gid))
}

func (f *LoopbackFile) ftruncate(sz uint64) syscall.Errno {
	return ToErrno(syscall.Ftruncate(f.fd, int64(sz)))
}

func (f *LoopbackFile) setAttr(ctx context.Context, in *fuse.SetAttrIn) syscall.Errno {
	var errno syscall.Errno
	if mode, ok := in.GetMode(); ok {
		if errno := f.fchmod(mode); errno != 0 {
			return errno
		}
	}

	uid32, uOk := in.GetUID()
	gid32, gOk := in.GetGID()
	if uOk || gOk {
		uid := -1
		gid := -1

		if uOk {
			uid = int(uid32)
		}
		if gOk {
			gid = int(gid32)
		}
		if errno := f.fchown(uid, gid); errno != 0 {
			return errno
		}
	}

	mtime, mok := in.GetMTime()
	atime, aok := in.GetATime()

	if mok || aok {
		ap := &atime
		mp := &mtime
		if !aok {
			ap = nil
		}
		if !mok {
			mp = nil
		}
		errno = f.utimens(ap, mp)
		if errno != 0 {
			return errno
		}
	}

	if sz, ok := in.GetSize(); ok {
		if errno := f.ftruncate(sz); errno != 0 {
			return errno
		}
	}
	return OK
}

func (f *LoopbackFile) Getattr(ctx context.Context, a *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := syscall.Stat_t{}
	err := syscall.Fstat(f.fd, &st)
	if err != nil {
		return ToErrno(err)
	}
	a.FromStat(&st)

	return OK
}

func (f *LoopbackFile) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := unix.Seek(f.fd, int64(off), int(whence))
	return uint64(n), ToErrno(err)
}

func (f *LoopbackFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := fallocate.Fallocate(f.fd, mode, int64(off), int64(sz))
	if err != nil {
		return ToErrno(err)
	}
	return OK
}

func (f *LoopbackFile) Ioctl(ctx context.Context, cmd uint32, arg uint64, input []byte, output []byte) (result int32, errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	argWord := uintptr(arg)
	ioc := ioctl.Command(cmd)
	if ioc.Read() {
		argWord = uintptr(unsafe.Pointer(&input[0]))
	} else if ioc.Write() {
		argWord = uintptr(unsafe.Pointer(&output[0]))
	}

	res, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(f.fd), uintptr(cmd), argWord)
	return int32(res), errno
}
