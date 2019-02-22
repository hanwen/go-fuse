// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	//	"time"
	"os"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

// LoopbackFile delegates all operations back to an underlying os.File.
func NewLoopbackFile(f *os.File) File {
	return &loopbackFile{File: f}
}

type loopbackFile struct {
	File *os.File

	// os.File is not threadsafe. Although fd themselves are
	// constant during the lifetime of an open file, the OS may
	// reuse the fd number after it is closed. When open races
	// with another close, they may lead to confusion as which
	// file gets written in the end.
	mu sync.Mutex
}

func (f *loopbackFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, code fuse.Status) {
	f.mu.Lock()
	// This is not racy by virtue of the kernel properly
	// synchronizing the open/write/close.
	r := fuse.ReadResultFd(f.File.Fd(), off, len(buf))
	f.mu.Unlock()
	return r, fuse.OK
}

func (f *loopbackFile) Write(ctx context.Context, data []byte, off int64) (uint32, fuse.Status) {
	f.mu.Lock()
	n, err := f.File.WriteAt(data, off)
	f.mu.Unlock()
	return uint32(n), fuse.ToStatus(err)
}

func (f *loopbackFile) Release(ctx context.Context) {
	f.mu.Lock()
	f.File.Close()
	f.mu.Unlock()
}

func (f *loopbackFile) Flush(ctx context.Context) fuse.Status {
	f.mu.Lock()

	// Since Flush() may be called for each dup'd fd, we don't
	// want to really close the file, we just want to flush. This
	// is achieved by closing a dup'd fd.
	newFd, err := syscall.Dup(int(f.File.Fd()))
	f.mu.Unlock()

	if err != nil {
		return fuse.ToStatus(err)
	}
	err = syscall.Close(newFd)
	return fuse.ToStatus(err)
}

func (f *loopbackFile) Fsync(ctx context.Context, flags int) (code fuse.Status) {
	f.mu.Lock()
	r := fuse.ToStatus(syscall.Fsync(int(f.File.Fd())))
	f.mu.Unlock()

	return r
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

func (f *loopbackFile) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status) {
	flk := syscall.Flock_t{}
	lk.ToFlockT(&flk)
	code = fuse.ToStatus(syscall.FcntlFlock(f.File.Fd(), _OFD_GETLK, &flk))
	out.FromFlockT(&flk)
	return
}

func (f *loopbackFile) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	return f.setLock(ctx, owner, lk, flags, false)
}

func (f *loopbackFile) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status) {
	return f.setLock(ctx, owner, lk, flags, true)
}

func (f *loopbackFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (code fuse.Status) {
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
			return fuse.EINVAL
		}
		if !blocking {
			op |= syscall.LOCK_NB
		}
		return fuse.ToStatus(syscall.Flock(int(f.File.Fd()), op))
	} else {
		flk := syscall.Flock_t{}
		lk.ToFlockT(&flk)
		var op int
		if blocking {
			op = _OFD_SETLKW
		} else {
			op = _OFD_SETLK
		}
		return fuse.ToStatus(syscall.FcntlFlock(f.File.Fd(), op, &flk))
	}
}

func (f *loopbackFile) Truncate(ctx context.Context, size uint64) fuse.Status {
	f.mu.Lock()
	r := fuse.ToStatus(syscall.Ftruncate(int(f.File.Fd()), int64(size)))
	f.mu.Unlock()

	return r
}

func (f *loopbackFile) Chmod(ctx context.Context, mode uint32) fuse.Status {
	f.mu.Lock()
	r := fuse.ToStatus(f.File.Chmod(os.FileMode(mode)))
	f.mu.Unlock()

	return r
}

func (f *loopbackFile) Chown(ctx context.Context, uid uint32, gid uint32) fuse.Status {
	f.mu.Lock()
	r := fuse.ToStatus(f.File.Chown(int(uid), int(gid)))
	f.mu.Unlock()

	return r
}

func (f *loopbackFile) GetAttr(ctx context.Context, a *fuse.Attr) fuse.Status {
	st := syscall.Stat_t{}
	f.mu.Lock()
	err := syscall.Fstat(int(f.File.Fd()), &st)
	f.mu.Unlock()
	if err != nil {
		return fuse.ToStatus(err)
	}
	a.FromStat(&st)

	return fuse.OK
}
