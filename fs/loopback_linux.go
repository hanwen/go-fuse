//go:build linux

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

const unix_UTIME_OMIT = unix.UTIME_OMIT

const dirOpenFlags = unix.O_PATH | unix.O_DIRECTORY

func aclKillSgid(ctx context.Context) bool {
	call, ok := fuse.CallFromContext(ctx)
	if !ok {
		return false
	}
	var in fuse.SetXAttrIn
	return fuse.Input(call, &in) && in.SetXAttrFlags&fuse.SETXATTR_ACL_KILL_SGID != 0
}

// openPath opens name without following it. Calls that do not accept
// O_PATH descriptors go through /proc/self/fd.
func openPath(dirfd int, name string) (int, string, error) {
	fd, err := unix.Openat(dirfd, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	return fd, fmt.Sprintf("/proc/self/fd/%d", fd), nil
}

// The underlying filesystem checks CAP_FSETID against our
// credentials, so clear setgid ourselves.
func setxattrKillSgid(dirfd int, name, attr string, data []byte, flags int) syscall.Errno {
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return ToErrno(err)
	}
	defer unix.Close(fd)
	if err := unix.Setxattr(p, attr, data, flags); err != nil {
		return ToErrno(err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return ToErrno(err)
	}
	if st.Mode&unix.S_ISGID != 0 {
		return ToErrno(unix.Chmod(p, st.Mode&07777&^unix.S_ISGID))
	}
	return OK
}

// noFchmodat2 and noXattrat record that the kernel lacks fchmodat2
// (6.6) or the *xattrat calls (6.13). Without them, we go through
// /proc/self/fd.
var noFchmodat2, noXattrat atomic.Bool

func fchmodatNoFollow(dirfd int, name string, mode uint32) error {
	if !noFchmodat2.Load() {
		err := fchmodat2(dirfd, name, mode, unix.AT_SYMLINK_NOFOLLOW)
		if err != unix.ENOSYS {
			return err
		}
		noFchmodat2.Store(true)
	}
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Chmod(p, mode)
}

func getxattrAt(dirfd int, name, attr string, dest []byte) (int, error) {
	if !noXattrat.Load() {
		sz, err := getxattrat(dirfd, name, unix.AT_SYMLINK_NOFOLLOW, attr, dest)
		if err != unix.ENOSYS {
			return sz, err
		}
		noXattrat.Store(true)
	}
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	return unix.Getxattr(p, attr, dest)
}

func setxattrAt(dirfd int, name, attr string, data []byte, flags int) error {
	if !noXattrat.Load() {
		err := setxattrat(dirfd, name, unix.AT_SYMLINK_NOFOLLOW, attr, data, flags)
		if err != unix.ENOSYS {
			return err
		}
		noXattrat.Store(true)
	}
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Setxattr(p, attr, data, flags)
}

func removexattrAt(dirfd int, name, attr string) error {
	if !noXattrat.Load() {
		err := removexattrat(dirfd, name, unix.AT_SYMLINK_NOFOLLOW, attr)
		if err != unix.ENOSYS {
			return err
		}
		noXattrat.Store(true)
	}
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Removexattr(p, attr)
}

func listxattrAt(dirfd int, name string, dest []byte) (int, error) {
	if !noXattrat.Load() {
		sz, err := listxattrat(dirfd, name, unix.AT_SYMLINK_NOFOLLOW, dest)
		if err != unix.ENOSYS {
			return sz, err
		}
		noXattrat.Store(true)
	}
	fd, p, err := openPath(dirfd, name)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	return unix.Listxattr(p, dest)
}

var _ = (NodeTmpfiler)((*LoopbackNode)(nil))

func (n *LoopbackNode) Tmpfile(ctx context.Context, flags uint32, mode uint32, out *fuse.EntryOut) (*Inode, FileHandle, uint32, syscall.Errno) {
	flags = flags &^ syscall.O_APPEND
	// openat2 rejects the file type bits that the kernel sends.
	fd, errno := n.RootData.openat(n.relativePath(), int(flags)|unix.O_TMPFILE, mode&07777)
	if errno != 0 {
		return nil, nil, 0, errno
	}

	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := fstatFd(fd, &st, &btime); err != nil {
		syscall.Close(fd)
		return nil, nil, 0, ToErrno(err)
	}

	node := n.RootData.newNode(n.EmbeddedInode(), "", &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))

	out.FromStat(&st)
	return ch, NewLoopbackFile(fd), 0, 0
}

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

func statAt(dirfd int, name string, st *syscall.Stat_t, btime *syscall.Timespec) error {
	return doStatx(dirfd, name, unix.AT_SYMLINK_NOFOLLOW, st, btime)
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
		// startup). Fall through to fstatat rather than failing
		// outright.
	}

	return unix.Fstatat(dirfd, path, (*unix.Stat_t)(unsafe.Pointer(st)), flags)
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

	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)

	st := unix.Statx_t{}
	err := unix.Statx(dirfd, name, int(flags)|unix.AT_SYMLINK_NOFOLLOW, int(mask), &st)
	if err != nil {
		return ToErrno(err)
	}
	out.FromStatx(&st)
	return OK
}
