// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"sync"
	"syscall"
	"unsafe"
)

func (ms *Server) systemWrite(req *request, header []byte) Status {
	if req.flatDataSize() == 0 {
		err := handleEINTR(func() error {
			_, err := syscall.Write(ms.mountFd, header)
			return err
		})
		return ToStatus(err)
	}

	if req.fdData != nil {
		sz := req.flatDataSize()
		buf := ms.allocOut(req, uint32(sz))
		req.flatData, req.status = req.fdData.Bytes(buf)
		header = req.serializeHeader(len(req.flatData))
	}

	_, err := writev(int(ms.mountFd), [][]byte{header, req.flatData})
	if req.readResult != nil {
		req.readResult.Done()
	}
	return ToStatus(err)
}

func readAll(fd int, dest []byte) (int, error) {
	offset := 0

	for offset < len(dest) {
		// read the remaining buffer
		err := handleEINTR(func() error {
			n, err := syscall.Read(fd, dest[offset:])
			if n == 0 && err == nil {
				// remote fd closed
				return syscall.EIO
			}
			offset += n
			return err
		})
		if err != nil {
			return offset, err
		}
	}
	return offset, nil
}

// for a stream connection we need to have a single reader
var readLock sync.Mutex

func (ms *Server) systemRead(dest []byte) (int, error) {
	var n int
	if osxFuse {
		err := handleEINTR(func() error {
			var err error
			n, err = syscall.Read(ms.mountFd, dest)
			return err
		})
		return n, err
	}

	readLock.Lock()
	defer readLock.Unlock()

	// read request length
	if _, err := readAll(ms.mountFd, dest[0:4]); err != nil {
		return 0, err
	}

	l := *(*uint32)(unsafe.Pointer(&dest[0]))
	// read remaining request
	if _, err := readAll(ms.mountFd, dest[4:l]); err != nil {
		return n, err
	}
	return int(l), nil
}
