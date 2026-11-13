// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fs

import (
	"context"
	"sync"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/ioctl"
	"golang.org/x/sys/unix"
)

// NewLoopbackDirStream opens a directory for reading as a DirStream
func NewLoopbackDirStream(name string) (DirStream, syscall.Errno) {
	// TODO: should return concrete type.
	fd, err := syscall.Open(name, syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0755)
	if err != nil {
		return nil, ToErrno(err)
	}
	return NewLoopbackDirStreamFd(fd)
}

type loopbackDirStream struct {
	buf []byte

	// Protects mutable members
	mu sync.Mutex

	// mutable
	todo      []byte
	todoErrno syscall.Errno
	fd        int
}

// NewLoopbackDirStreamFd reads the directory opened at file descriptor fd as
// a DirStream
func NewLoopbackDirStreamFd(fd int) (DirStream, syscall.Errno) {
	ds := &loopbackDirStream{
		fd: fd,
	}
	return ds, OK
}

func (ds *loopbackDirStream) Close() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.fd != -1 {
		syscall.Close(ds.fd)
		ds.fd = -1
	}
}

var _ = (FileReleasedirer)((*loopbackDirStream)(nil))

func (ds *loopbackDirStream) Releasedir(ctx context.Context, flags uint32) {
	ds.Close()
}

var _ = (FileSeekdirer)((*loopbackDirStream)(nil))

func (ds *loopbackDirStream) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	_, errno := unix.Seek(ds.fd, int64(off), unix.SEEK_SET)
	if errno != nil {
		return ToErrno(errno)
	}

	ds.todo = nil
	ds.todoErrno = 0
	ds.load()
	return 0
}

var _ = (FileFsyncdirer)((*loopbackDirStream)(nil))

func (ds *loopbackDirStream) Fsyncdir(ctx context.Context, flags uint32) syscall.Errno {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ToErrno(syscall.Fsync(ds.fd))
}

func (ds *loopbackDirStream) HasNext() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.load()
	return len(ds.todo) > 0 || ds.todoErrno != 0
}

var _ = (FileReaddirenter)((*loopbackDirStream)(nil))

func (ds *loopbackDirStream) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if !ds.HasNext() {
		return nil, 0
	}
	de, errno := ds.Next()
	return &de, errno
}

func (ds *loopbackDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.load()
	if ds.todoErrno != 0 {
		return fuse.DirEntry{}, ds.todoErrno
	}
	var res fuse.DirEntry
	n := res.Parse(ds.todo)
	ds.todo = ds.todo[n:]
	return res, 0
}

var _ = (FileIoctler)((*loopbackDirStream)(nil))

func (ds *loopbackDirStream) Ioctl(ctx context.Context, cmd uint32, arg uint64, input []byte, output []byte) (result int32, errno syscall.Errno) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	argWord := uintptr(arg)
	ioc := ioctl.Command(cmd)
	if ioc.Read() && ioc.Write() {
		// The kernel updates the buffer in place.
		copy(output, input)
		argWord = uintptr(unsafe.Pointer(&output[0]))
	} else if ioc.Write() {
		argWord = uintptr(unsafe.Pointer(&input[0]))
	} else if ioc.Read() {
		argWord = uintptr(unsafe.Pointer(&output[0]))
	}

	res, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ds.fd), uintptr(cmd), argWord)
	return int32(res), errno
}

func (ds *loopbackDirStream) load() {
	if len(ds.todo) > 0 || ds.todoErrno != 0 {
		return
	}
	if ds.buf == nil {
		ds.buf = make([]byte, 4096)
	}
	n, err := getdents(ds.fd, ds.buf)
	if n < 0 {
		n = 0
	}
	ds.todo = ds.buf[:n]
	ds.todoErrno = ToErrno(err)
}
