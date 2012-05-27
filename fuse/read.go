package fuse

import (
	"io"
	"syscall"
)

// The result of Read is an array of bytes, but for performance
// reasons, we can also return data as a file-descriptor/offset/size
// tuple.  If the backing store for a file is another filesystem, this
// reduces the amount of copying between the kernel and the FUSE
// server
//
// If at any point,  the raw data is needed, ReadResult.Read() will
// load the raw data into the Data member.
type ReadResult struct {
	// Raw bytes for the read.
	Data []byte

	// If Data is nil and Status OK, splice from the following
	// file.
	Fd uintptr

	// Offset within Fd, or -1 to use current offset.
	FdOff  int64

	// Size of data to be loaded. Actual data available may be
	// less at the EOF.
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

// Reads raw bytes from file descriptor if necessary, using the passed
// buffer as storage.
func (r *ReadResult) Read(buf []byte) Status {
	if r.Data != nil {
		return OK
	}
	if len(buf) < r.FdSize {
		return ERANGE
	}

	n, err := syscall.Pread(int(r.Fd), buf[:r.FdSize], r.FdOff)
	if err == io.EOF {
		err = nil
	}
	code := ToStatus(err)
	if code.Ok() {
		r.Data = buf[:n]
	}
	r.Fd = 0
	r.FdOff = 0
	r.FdSize = 0

	return code
}
