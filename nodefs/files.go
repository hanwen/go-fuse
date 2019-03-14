// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	//	"time"

	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

func newLoopbackFile(fd int) *loopbackFile {
	return &loopbackFile{fd: fd}
}

// loopbackFile delegates all operations back to an underlying file.
type loopbackFile struct {
	fd int

	// Although fd themselves are constant during the lifetime of
	// an open file, the OS may reuse the fd number after it is
	// closed. When open races with another close, they may lead
	// to confusion as which file gets written in the end.
	mu sync.Mutex
}

func (f *loopbackFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, status fuse.Status) {
	f.mu.Lock()
	// This is not racy by virtue of the kernel properly
	// synchronizing the open/write/close.
	r := fuse.ReadResultFd(uintptr(f.fd), off, len(buf))
	f.mu.Unlock()
	return r, fuse.OK
}

func (f *loopbackFile) Write(ctx context.Context, data []byte, off int64) (uint32, fuse.Status) {
	f.mu.Lock()
	n, err := syscall.Pwrite(f.fd, data, off)
	f.mu.Unlock()
	return uint32(n), fuse.ToStatus(err)
}

func (f *loopbackFile) Release(ctx context.Context) fuse.Status {
	f.mu.Lock()
	err := syscall.Close(f.fd)
	f.mu.Unlock()
	return fuse.ToStatus(err)
}

func (f *loopbackFile) Flush(ctx context.Context) fuse.Status {
	f.mu.Lock()

	// Since Flush() may be called for each dup'd fd, we don't
	// want to really close the file, we just want to flush. This
	// is achieved by closing a dup'd fd.
	newFd, err := syscall.Dup(f.fd)
	f.mu.Unlock()

	if err != nil {
		return fuse.ToStatus(err)
	}
	err = syscall.Close(newFd)
	return fuse.ToStatus(err)
}

func (f *loopbackFile) Fsync(ctx context.Context, flags uint32) (status fuse.Status) {
	f.mu.Lock()
	r := fuse.ToStatus(syscall.Fsync(f.fd))
	f.mu.Unlock()

	return r
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

func (f *loopbackFile) GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (status fuse.Status) {
	flk := syscall.Flock_t{}
	lk.ToFlockT(&flk)
	status = fuse.ToStatus(syscall.FcntlFlock(uintptr(f.fd), _OFD_GETLK, &flk))
	out.FromFlockT(&flk)
	return
}

func (f *loopbackFile) SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return f.setLock(ctx, owner, lk, flags, false)
}

func (f *loopbackFile) SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (status fuse.Status) {
	return f.setLock(ctx, owner, lk, flags, true)
}

func (f *loopbackFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (status fuse.Status) {
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
		return fuse.ToStatus(syscall.Flock(f.fd, op))
	} else {
		flk := syscall.Flock_t{}
		lk.ToFlockT(&flk)
		var op int
		if blocking {
			op = _OFD_SETLKW
		} else {
			op = _OFD_SETLK
		}
		return fuse.ToStatus(syscall.FcntlFlock(uintptr(f.fd), op, &flk))
	}
}

func (f *loopbackFile) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	if status := f.setAttr(ctx, in); !status.Ok() {
		return status
	}

	return f.GetAttr(ctx, out)
}

func (f *loopbackFile) setAttr(ctx context.Context, in *fuse.SetAttrIn) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	var status fuse.Status
	if mode, ok := in.GetMode(); ok {
		status = fuse.ToStatus(syscall.Fchmod(f.fd, mode))
		if !status.Ok() {
			return status
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
		status = fuse.ToStatus(syscall.Fchown(f.fd, uid, gid))
		if !status.Ok() {
			return status
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
		status = f.utimens(ap, mp)
		if !status.Ok() {
			return status
		}
	}

	if sz, ok := in.GetSize(); ok {
		status = fuse.ToStatus(syscall.Ftruncate(f.fd, int64(sz)))
		if !status.Ok() {
			return status
		}
	}
	return fuse.OK
}

func (f *loopbackFile) GetAttr(ctx context.Context, a *fuse.AttrOut) fuse.Status {
	st := syscall.Stat_t{}
	f.mu.Lock()
	err := syscall.Fstat(f.fd, &st)
	f.mu.Unlock()
	if err != nil {
		return fuse.ToStatus(err)
	}
	a.FromStat(&st)

	return fuse.OK
}
