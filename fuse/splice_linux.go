// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"errors"
	"fmt"
	"os"

	"github.com/hanwen/go-fuse/v2/splice"
)

// errRecoverSplice is returned by trySplice when the caller should
// fall back to to pread/read without logging.
var errRecoverSplice = errors.New("splice failed; must fallback")

func (s *Server) setSplice() {
	s.canSplice = splice.Resizable() && !s.opts.DisableSplice
}

// trySplice: Zero-copy read from fdData.Fd into /dev/fuse
//
// Optimistic fast path: assumes fdData.Sz bytes are available (no short
// read). The caller has already serialized the reply header with that size.
// File data is copied into a pipe only once:
//
//  1. Write pre-serialized header into pipe  --> pipe: [header]
//  2. Splice file data directly into pipe    --> pipe: [header][payload]
//  3. Splice pipe into /dev/fuse
//
// If a short read occurs (payloadLen < fdData.Sz), the header in the pipe
// would carry the wrong total length, so we return an error and let the
// caller fall back to a Pread-based path.
func (ms *Server) trySplice(req *request, readResult ReadResult) error {
	// The caller (handleRequest) already called req.serializeHeader with
	// readResult.Size(), so req.outHeaderBuf is correct for the optimistic case.
	total := len(req.outHeaderBuf) + len(req.outDataBuf) + readResult.Size()

	pair, err := splice.Get()
	if err != nil {
		return err
	}
	defer splice.Done(pair)

	// Grow pipe to header + payload + one extra page.
	// Without the extra page the kernel will block once the pipe is almost full.
	if err := pair.Grow(total + os.Getpagesize()); err != nil {
		return err
	}

	// Write header into pipe.
	n, err := writev(int(pair.WriteFd()), [][]byte{req.outHeaderBuf, req.outDataBuf})
	if err != nil {
		return err
	}
	if want := len(req.outHeaderBuf) + len(req.outDataBuf); n != want {
		return fmt.Errorf("short write into splice: wrote %d, want %d", n, want)
	}

	// Splice file data directly into pipe (single copy).
	var payloadLen int
	var fd uintptr
	var sz int
	var off int64
	if seekable, ok := readResult.(seekableResult); ok {
		fd, off, sz = seekable.Seekable()
		payloadLen, err = pair.LoadFromAt(fd, sz, off)
	} else if stateful, ok := readResult.(statefulResult); ok {
		fd, sz = stateful.Stateful()
		payloadLen, err = pair.LoadFrom(fd, sz)
	} else {
		return errRecoverSplice
	}

	if err != nil {
		return err
	}

	if payloadLen != sz {
		// Short read at EOF: the header carries the wrong total length.
		// Return errShortSplice so the caller falls back without logging.
		return errRecoverSplice
	}

	// Write header + payload to /dev/fuse.
	_, err = pair.WriteTo(uintptr(ms.mountFd), total)
	return err
}
