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

// errShortSplice is returned by trySplice when the file had fewer bytes than
// expected (EOF short read). The caller should fall back to Pread without
// logging, since this is a normal condition for files whose size changes.
var errShortSplice = errors.New("short splice")

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
func (ms *Server) trySplice(req *request, fdData *readResultFd) error {
	// The caller (handleRequest) already called req.serializeHeader with
	// fdData.Sz, so req.outHeaderBuf is correct for the optimistic case.
	total := len(req.outHeaderBuf) + len(req.outDataBuf) + fdData.Sz

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
	payloadLen, err := pair.LoadFromAt(fdData.Fd, fdData.Sz, fdData.Off)
	if err != nil {
		return err
	}
	if payloadLen != fdData.Sz {
		// Short read at EOF: the header carries the wrong total length.
		// Return errShortSplice so the caller falls back without logging.
		return errShortSplice
	}

	// Write header + payload to /dev/fuse.
	_, err = pair.WriteTo(uintptr(ms.mountFd), total)
	return err
}
