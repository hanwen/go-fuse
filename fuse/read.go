package fuse

import (
	"io"
	"syscall"
)

type ReadResult struct {
	Status
	Data []byte

	// If Data is nil and Status OK, splice from the following
	// file.
	Fd   uintptr

	// Offset within Fd, or -1 to use current offset.
	FdOff  int64
	FdSize int
}

func (r *ReadResult) Clear() {
	*r = ReadResult{}
}

func (r *ReadResult) Size() int {
	if r.Data != nil {
		return len(r.Data)
	}
	return r.FdSize
}

func (r *ReadResult) Read(buf []byte) Status {
	if r.Data != nil || !r.Ok() {
		return r.Status
	}
	if len(buf) < r.FdSize {
		r.Status = ERANGE
		return ERANGE
	}

	n, err := syscall.Pread(int(r.Fd), buf[:r.FdSize], r.FdOff)
	if err == io.EOF {
		err = nil
	}
	r.Status = ToStatus(err)
	if r.Ok() {
		r.Data = buf[:n]
	}
	r.Fd = 0
	r.FdOff = 0
	r.FdSize = 0

	return r.Status
}
