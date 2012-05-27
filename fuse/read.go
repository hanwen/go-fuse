package fuse

import (
	"io"
	"syscall"
)

type ReadResult interface {
	Bytes(buf []byte) []byte
	Size() int
}

// The result of Read is an array of bytes, but for performance
// reasons, we can also return data as a file-descriptor/offset/size
// tuple.  If the backing store for a file is another filesystem, this
// reduces the amount of copying between the kernel and the FUSE
// server
//
// If at any point,  the raw data is needed, ReadResult.Read() will
// load the raw data into the Data member.
type ReadResultData struct {
	// Raw bytes for the read.
	Data []byte
}

func (r *ReadResultData) Size() int {
	return len(r.Data)
}

func (r *ReadResultData) Bytes(buf []byte) []byte {
	return r.Data
}

type ReadResultFd struct {
	// If Data is nil and Status OK, splice from the following
	// file.
	Fd uintptr

	// Offset within Fd, or -1 to use current offset.
	Off  int64

	// Size of data to be loaded. Actual data available may be
	// less at the EOF.
	Sz int
}

// Reads raw bytes from file descriptor if necessary, using the passed
// buffer as storage.
func (r *ReadResultFd) Bytes(buf []byte) []byte {
	sz := r.Sz
	if len(buf) < sz {
		sz = len(buf)
	}

	n, err := syscall.Pread(int(r.Fd), buf[:sz], r.Off)
	if err == io.EOF {
		err = nil
	}
	// TODO - error handling?
	return buf[:n]
}

func (r *ReadResultFd) Size() int {
	return r.Sz
}

