// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"io"
	"syscall"
)

// ReadResultData is the read return for returning bytes directly.
type readResultData struct {
	// Raw bytes for the read.
	Data []byte
}

func (r *readResultData) Size() int {
	return len(r.Data)
}

func (r *readResultData) Done() {
}

func (r *readResultData) Bytes(buf []byte) ([]byte, Status) {
	return r.Data, OK
}

func ReadResultData(b []byte) ReadResult {
	return &readResultData{b}
}

func ReadResultFd(fd uintptr, off int64, sz int) ReadResult {
	return &readResultFd{fd, off, sz}
}

type seekableResult interface {
	Seekable() (fd uintptr, off int64, sz int)
}

type statefulResult interface {
	Stateful() (fd uintptr, sz int)
}

type withSlice interface {
	// Slices may be called more than once and must return the same data each time.
	Slices() ([][]byte, Status)
}

// withSpliceFlags is a ReadResult carrying splice(2) flags for the write to
// /dev/fuse.
type withSpliceFlags interface {
	SpliceFlags() int
}

// ReadResultFd is the read return for zero-copy file data.
type readResultFd struct {
	// Splice from the following file.
	Fd uintptr

	// Offset within Fd, or -1 to use current offset.
	Off int64

	// Size of data to be loaded. Actual data available may be
	// less at the EOF.
	Sz int
}

func (r *readResultFd) Seekable() (fd uintptr, off int64, sz int) {
	return r.Fd, r.Off, r.Sz
}

// Reads raw bytes from file descriptor if necessary, using the passed
// buffer as storage.
func (r *readResultFd) Bytes(buf []byte) ([]byte, Status) {
	sz := min(len(buf), r.Sz)

	n, err := syscall.Pread(int(r.Fd), buf[:sz], r.Off)
	if err == io.EOF {
		err = nil
	}

	if n < 0 {
		n = 0
	}

	return buf[:n], ToStatus(err)
}

func (r *readResultFd) Size() int {
	return r.Sz
}

func (r *readResultFd) Done() {
}

// readResultVector is the read return for scatter-gather I/O. It implements
// the withSlice interface so the kernel write uses writev(2) directly,
// avoiding a copy into a single contiguous buffer.
type readResultVector struct {
	vecs [][]byte
}

func (r *readResultVector) Size() int {
	return iovLen(r.vecs)
}

func (r *readResultVector) Done() {}

// Bytes concatenates the vector into buf, reusing its capacity.
func (r *readResultVector) Bytes(buf []byte) ([]byte, Status) {
	buf = buf[:0]
	for _, v := range r.vecs {
		buf = append(buf, v...)
	}
	return buf, OK
}

func (r *readResultVector) Slices() ([][]byte, Status) {
	return r.vecs, OK
}

// ReadResultVector returns a ReadResult for scatter-gather I/O.
// When the kernel write path supports it, the buffers are sent via
// writev(2) without being copied into a single contiguous region.
func ReadResultVector(vecs [][]byte) ReadResult {
	return &readResultVector{vecs}
}
