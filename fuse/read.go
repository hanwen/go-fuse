package fuse

import (
	"io"
	"syscall"
)

// ReadResultData is the read return for returning bytes directly.
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

// ReadResultFd is the read return for zero-copy file data.
type ReadResultFd struct {
	// Splice from the following file.
	Fd uintptr

	// Offset within Fd, or -1 to use current offset.
	Off int64

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
