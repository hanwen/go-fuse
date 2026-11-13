// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"io"
	"syscall"
)

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
