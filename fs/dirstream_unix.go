//go:build !darwin

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

type loopbackDirStream struct {
	buf []byte

	// Protects mutable members
	mu sync.Mutex

	// mutable
	todo      []byte
	todoErrno syscall.Errno
	fd        int
}

// NewLoopbackDirStream open a directory for reading as a DirStream
func NewLoopbackDirStream(name string) (DirStream, syscall.Errno) {
	// TODO: should return concrete type.
	fd, err := syscall.Open(name, syscall.O_DIRECTORY, 0755)
	if err != nil {
		return nil, ToErrno(err)
	}

	ds := &loopbackDirStream{
		buf: make([]byte, 4096),
		fd:  fd,
	}

	ds.load()
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

func (ds *loopbackDirStream) HasNext() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return len(ds.todo) > 0 || ds.todoErrno != 0
}

func (ds *loopbackDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.todoErrno != 0 {
		return fuse.DirEntry{}, ds.todoErrno
	}
	var res fuse.DirEntry
	n := res.Parse(ds.todo)
	ds.todo = ds.todo[n:]
	if len(ds.todo) == 0 {
		ds.load()
	}
	return res, 0
}

func (ds *loopbackDirStream) load() {
	if len(ds.todo) > 0 {
		return
	}

	n, err := unix.Getdents(ds.fd, ds.buf)
	if n < 0 {
		n = 0
	}
	ds.todo = ds.buf[:n]
	ds.todoErrno = ToErrno(err)
}
