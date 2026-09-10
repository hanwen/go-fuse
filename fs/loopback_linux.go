//go:build linux

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

const unix_UTIME_OMIT = unix.UTIME_OMIT

func doCopyFileRange(fdIn int, offIn int64, fdOut int, offOut int64,
	len int, flags int) (uint32, syscall.Errno) {
	count, err := unix.CopyFileRange(fdIn, &offIn, fdOut, &offOut, len, flags)
	return uint32(count), ToErrno(err)
}

func intDev(dev uint32) int {
	return int(dev)
}

var statxSupported bool

func init() {
	var x unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, "/", 0, unix.STATX_BASIC_STATS, &x)
	statxSupported = err != unix.ENOSYS
}

func lstat(path string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	return doStatx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, st, btime)
}

func stat(path string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	return doStatx(unix.AT_FDCWD, path, 0, st, btime)
}

func fstatFd(fd int, st *syscall.Stat_t, btime *syscall.Timespec) error {
	return doStatx(fd, "", unix.AT_EMPTY_PATH, st, btime)
}

func doStatx(dirfd int, path string, flags int, st *syscall.Stat_t, btime *syscall.Timespec) error {
	*btime = syscall.Timespec{}

	if statxSupported {
		var x unix.Statx_t
		err := unix.Statx(dirfd, path, flags, unix.STATX_BASIC_STATS|unix.STATX_BTIME, &x)
		if err == nil {
			statFromStatx(&x, st)
			if x.Mask&unix.STATX_BTIME != 0 {
				*btime = statxTimeToTimespec(x.Btime)
			}
			return nil
		}
		if err != unix.ENOSYS {
			return err
		}
		// The init probe found statx(2) supported, but this particular
		// call still says ENOSYS (e.g. a seccomp filter tightened after
		// startup). Fall through to the plain syscalls below rather than
		// failing outright.
	}

	switch {
	case flags&unix.AT_EMPTY_PATH != 0:
		return syscall.Fstat(dirfd, st)
	case flags&unix.AT_SYMLINK_NOFOLLOW != 0:
		return syscall.Lstat(path, st)
	default:
		return syscall.Stat(path, st)
	}
}

// setNum assigns v to *dst, narrowing/widening to dst's element type.
func setNum[T ~int16 | ~int32 | ~int64 | ~uint16 | ~uint32 | ~uint64](dst *T, v int64) {
	*dst = T(v)
}

func statxTimeToTimespec(ts unix.StatxTimestamp) syscall.Timespec {
	return syscall.Timespec(unix.NsecToTimespec(ts.Sec*1e9 + int64(ts.Nsec)))
}

// statFromStatx fills st from a unix.Statx_t, so it can be used everywhere
// a Stat_t is expected (idFromStat, fuse.Attr.FromStat).
func statFromStatx(x *unix.Statx_t, st *syscall.Stat_t) {
	// use setNum; statx widths differ per architecture.
	setNum(&st.Dev, int64(unix.Mkdev(x.Dev_major, x.Dev_minor)))
	setNum(&st.Ino, int64(x.Ino))
	setNum(&st.Nlink, int64(x.Nlink))
	setNum(&st.Mode, int64(x.Mode))
	setNum(&st.Uid, int64(x.Uid))
	setNum(&st.Gid, int64(x.Gid))
	setNum(&st.Rdev, int64(unix.Mkdev(x.Rdev_major, x.Rdev_minor)))
	setNum(&st.Size, int64(x.Size))
	setNum(&st.Blksize, int64(x.Blksize))
	setNum(&st.Blocks, int64(x.Blocks))
	st.Atim = statxTimeToTimespec(x.Atime)
	st.Mtim = statxTimeToTimespec(x.Mtime)
	st.Ctim = statxTimeToTimespec(x.Ctime)
}

var _ = (NodeStatxer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Statx(ctx context.Context, f FileHandle,
	flags uint32, mask uint32,
	out *fuse.StatxOut) syscall.Errno {
	if f != nil {
		if fga, ok := f.(FileStatxer); ok {
			return fga.Statx(ctx, flags, mask, out)
		}
	}

	p := n.path()

	st := unix.Statx_t{}
	err := unix.Statx(unix.AT_FDCWD, p, int(flags), int(mask), &st)
	if err != nil {
		return ToErrno(err)
	}
	out.FromStatx(&st)
	return OK
}
