// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "unsafe"

// FUSE-over-io_uring ABI mirrors.
//
// Source of truth: include/uapi/linux/fuse.h in Linux ≥6.10. Layouts here
// must match the kernel byte-for-byte; the kernel reads/writes these
// structures directly through the mmap'd uring entry buffer.

// fuseUringReqHeaderSize is FUSE_URING_REQ_HEADER_SZ from linux/fuse.h.
// Both the in/out header region and the op-specific region of a uring
// entry are this size.
const fuseUringReqHeaderSize = 128

// FUSE_IO_URING_CMD_* cmd_op values for IORING_OP_URING_CMD SQEs targeting
// a /dev/fuse fd.
const (
	_FUSE_IO_URING_CMD_INVALID           = 0
	_FUSE_IO_URING_CMD_REGISTER          = 1
	_FUSE_IO_URING_CMD_COMMIT_AND_FETCH  = 2
)

// fuseUringEntInOut is the trailing metadata block of a uring entry's
// header region. The kernel populates payload_sz on FETCH completion and
// reads it back on COMMIT_AND_FETCH.
//
// Mirrors struct fuse_uring_ent_in_out.
type fuseUringEntInOut struct {
	Flags     uint64
	CommitID  uint64
	PayloadSz uint32
	_         uint32 // padding
	_         uint64 // reserved
}

// fuseUringReqHeader is the fixed-size header at the start of each uring
// entry's pre-mapped buffer. The variable-size request/reply payload
// follows in the entry's payload region (registered separately).
//
// Mirrors struct fuse_uring_req_header.
type fuseUringReqHeader struct {
	// InOut overlays either a FUSE InHeader+inData (request) or an
	// OutHeader+outData (reply), depending on direction.
	InOut [fuseUringReqHeaderSize]byte

	// OpIn is op-specific request data — for example a ReadIn for
	// FUSE_READ. For replies this region is unused.
	OpIn [fuseUringReqHeaderSize]byte

	RingEntInOut fuseUringEntInOut
}

// fuseUringCmdReq is the SQE cmd payload (sqe.cmd[..]) for
// FUSE_IO_URING_CMD_{REGISTER,COMMIT_AND_FETCH}. Mirrors struct
// fuse_uring_cmd_req.
type fuseUringCmdReq struct {
	Flags    uint64
	CommitID uint64
	Qid      uint16
	_        [6]byte // padding
}

// Static size assertions: any drift from the kernel ABI fails to compile.
// Sizes are taken from include/uapi/linux/fuse.h.
const (
	_ uint = uint(unsafe.Sizeof(fuseUringEntInOut{})) - 32
	_ uint = 32 - uint(unsafe.Sizeof(fuseUringEntInOut{}))

	_ uint = uint(unsafe.Sizeof(fuseUringReqHeader{})) - (2*fuseUringReqHeaderSize + 32)
	_ uint = (2*fuseUringReqHeaderSize + 32) - uint(unsafe.Sizeof(fuseUringReqHeader{}))

	_ uint = uint(unsafe.Sizeof(fuseUringCmdReq{})) - 24
	_ uint = 24 - uint(unsafe.Sizeof(fuseUringCmdReq{}))
)

