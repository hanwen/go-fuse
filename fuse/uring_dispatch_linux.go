// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

// FUSE-over-io_uring dispatch loop. One uringQueue wraps a single io_uring
// ring (one per CPU in production), holds the per-entry buffer regions,
// and runs the fetch/commit-and-fetch cycle that exchanges FUSE requests
// and replies with the kernel.
//
// The transport-agnostic io_uring plumbing lives in uring_core_linux.go.
// The FUSE-side ABI lives in uring_linux.go.

import (
	"fmt"
	"syscall"
	"unsafe"
)

// uringEntry is one slot in the per-CPU queue. The header and payload
// regions are allocated once and registered with the kernel; for the
// lifetime of the queue the kernel reads requests into them and reads
// replies out of them.
type uringEntry struct {
	header  *fuseUringReqHeader
	payload []byte
	// iov is the two-element iovec the kernel reads at REGISTER time to
	// learn the buffer addresses. Kept alive on the entry to stop the
	// GC from moving it.
	iov [2]syscall.Iovec

	// opOutScratch is a private write target for the op-specific
	// reply struct (EntryOut, AttrOut, ...). The kernel reads the
	// reply from e.payload, but request and reply alias there: the
	// kernel writes the LOOKUP name into payload, and handlers expect
	// a zero-init output buffer per the go-fuse API contract. We let
	// the handler write into this scratch (zeroed each cycle), then
	// memcpy it into payload before COMMIT_AND_FETCH.
	opOutScratch [outputDataSize]byte
}

// uringQueue owns one io_uring instance plus its per-entry buffer set.
type uringQueue struct {
	ring     *uringRing
	fuseFd   int
	qid      uint16
	entries  []uringEntry
	ps       *protocolServer
	stopping uint32 // atomic; non-zero means drain and exit

	// debugf is optional. When set, the dispatch loop traces its
	// activity for diagnosis of ABI/protocol mismatches.
	debugf func(format string, args ...any)
}

const (
	// uringDefaultEntries is the number of in-flight requests per ring.
	// One ring per CPU; total in-flight requests = NCPU * this.
	uringDefaultEntries = 4
)

// newUringQueue creates a queue for one CPU/qid. The entry payload size
// is set from the negotiated MaxWrite plus a small request margin.
func newUringQueue(ps *protocolServer, fuseFd int, qid uint16, entries uint32, payloadSize int) (*uringQueue, error) {
	flags := uint32(_IORING_SETUP_SQE128)
	r, err := newUringRing(entries, flags)
	if err != nil {
		return nil, err
	}
	q := &uringQueue{
		ring:    r,
		fuseFd:  fuseFd,
		qid:     qid,
		entries: make([]uringEntry, entries),
		ps:      ps,
	}
	for i := range q.entries {
		e := &q.entries[i]
		e.header = new(fuseUringReqHeader)
		e.payload = make([]byte, payloadSize)
		e.iov[0] = syscall.Iovec{Base: (*byte)(unsafe.Pointer(e.header))}
		e.iov[0].SetLen(int(unsafe.Sizeof(fuseUringReqHeader{})))
		e.iov[1] = syscall.Iovec{Base: &e.payload[0]}
		e.iov[1].SetLen(len(e.payload))
	}
	return q, nil
}

// submitFetch submits a FUSE_IO_URING_CMD_REGISTER SQE for entry index i.
// REGISTER advertises the entry buffer and parks the SQE in the kernel
// until a FUSE request arrives, at which point the CQE fires.
func (q *uringQueue) submitFetch(i int) error {
	sqe := q.ring.nextSqe()
	if sqe == nil {
		return fmt.Errorf("SQ full")
	}
	q.fillUringCmd(sqe, i, _FUSE_IO_URING_CMD_REGISTER, 0)
	q.ring.commitSqe()
	return nil
}

// submitCommitAndFetch publishes the reply for entry i (using its
// commit_id, set on the previous CQE) and re-arms the slot.
func (q *uringQueue) submitCommitAndFetch(i int, commitID uint64) error {
	sqe := q.ring.nextSqe()
	if sqe == nil {
		return fmt.Errorf("SQ full")
	}
	q.fillUringCmd(sqe, i, _FUSE_IO_URING_CMD_COMMIT_AND_FETCH, commitID)
	q.ring.commitSqe()
	return nil
}

func (q *uringQueue) fillUringCmd(sqe *ioUringSqe, entryIdx int, cmdOp uint32, commitID uint64) {
	e := &q.entries[entryIdx]
	sqe.Opcode = _IORING_OP_URING_CMD
	sqe.Fd = int32(q.fuseFd)
	// For URING_CMD, cmd_op lives in the low 32 bits of the SQE Off
	// union.
	sqe.Off = uint64(cmdOp)
	// addr/len describe the iovec the kernel reads at REGISTER (and
	// references on subsequent FETCH cycles).
	sqe.Addr = uint64(uintptr(unsafe.Pointer(&e.iov[0])))
	sqe.Len = 2
	// UserData carries the entry index so the completion side can find
	// it again. qid is implicit per-ring.
	sqe.UserData = uint64(entryIdx)

	// Fill struct fuse_uring_cmd_req in the SQE cmd[] payload.
	req := fuseUringCmdReq{CommitID: commitID, Qid: q.qid}
	dst := sqe.CmdPayload()
	*(*fuseUringCmdReq)(unsafe.Pointer(&dst[0])) = req
}

func (q *uringQueue) tracef(format string, args ...any) {
	if q.debugf != nil {
		q.debugf("uring q%d: "+format, append([]any{q.qid}, args...)...)
	}
}

// Run drives the fetch/commit cycle until stop is signalled. It is
// expected to run on its own goroutine, ideally pinned to a CPU
// corresponding to qid.
//
// Invariant: every loop iteration calls ioUringEnter exactly once with
// (pending, 1, GETEVENTS). This atomically submits all SQEs queued by
// the previous iteration's handlers AND waits for at least one new
// completion. Splitting submit and wait races: if a request lands while
// we are about to wait without having flushed our reply SQEs, the
// kernel waits for us and we wait for the kernel — deadlock.
func (q *uringQueue) Run() error {
	for i := range q.entries {
		if err := q.submitFetch(i); err != nil {
			return err
		}
	}
	pending := uint32(len(q.entries))
	q.tracef("priming %d REGISTER SQEs", pending)

	for {
		_, err := ioUringEnter(q.ring.fd, pending, 1, _IORING_ENTER_GETEVENTS)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return fmt.Errorf("uring_enter: %w", err)
		}
		pending = 0

		for {
			cqe := q.ring.peekCqe()
			if cqe == nil {
				break
			}
			idx := int(cqe.UserData)
			res := cqe.Res
			q.ring.advanceCq()

			if res < 0 {
				// -ENOTCONN / -ENODEV — kernel is tearing down
				// the ring. Exit the loop.
				q.tracef("CQE entry=%d errno=%d, exiting", idx, -res)
				return syscall.Errno(-res)
			}
			q.tracef("CQE entry=%d res=%d", idx, res)

			if err := q.handle(idx); err != nil {
				return err
			}
			pending++
		}
	}
}

// handle dispatches the request currently sitting in entry idx, then
// re-submits the entry as COMMIT_AND_FETCH.
//
// Buffer layout per the kernel (fs/fuse/dev_uring.c):
//
// REQUEST (kernel → server):
//   - in_out[..sizeof(InHeader)]            = InHeader
//   - op_in[..in_args[0].size]              = op-specific input struct
//   - payload[..ring_ent_in_out.payload_sz] = remaining in_args (e.g. name)
//
// REPLY (server → kernel):
//   - in_out[..sizeof(OutHeader)]              = OutHeader (only 16 bytes)
//   - payload[..ring_ent_in_out.payload_sz]    = all out_args concatenated
//     (op-specific reply struct first, then trailing data)
//   - op_in is NOT read on the reply side.
func (q *uringQueue) handle(idx int) error {
	e := &q.entries[idx]

	headerSlice := e.header.InOut[:]
	inHdrLen := int(unsafe.Sizeof(InHeader{}))

	// Pre-parse to learn outSize/outPayloadSize so we can carve the
	// reply payload into the op-out slot + trailing data slot. The
	// inBuf concatenation matches what parseRequest expects: InHeader
	// at offset 0, op-specific input struct at offset inHdrLen.
	inBuf := make([]byte, inHdrLen+len(e.header.OpIn))
	copy(inBuf, headerSlice[:inHdrLen])
	copy(inBuf[inHdrLen:], e.header.OpIn[:])
	h, inSize, outSize, outPayloadSize, errno := parseRequest(inBuf, &q.ps.kernelSettings)
	if errno != OK {
		q.tracef("parseRequest entry=%d errno=%v", idx, errno)
		return q.commitError(idx, errno)
	}
	q.tracef("op %s entry=%d outSize=%d outPayloadSize=%d", h.Name, idx, outSize, outPayloadSize)

	// Trim in[1] to the actual op-specific input size. The op_in slot
	// is 128 bytes, but only the first (inSize - sizeof(InHeader)) bytes
	// were written by the kernel; the rest is uninitialized zero
	// padding. Without the trim it gets interpreted as inPayload.
	opInLen := inSize - inHdrLen
	if opInLen < 0 {
		opInLen = 0
	}
	inSz := e.header.RingEntInOut.PayloadSz
	// Pass inBuf (our own copy of InHeader+op_in) rather than the
	// live e.header.InOut slice: handleIov re-copies in[0]/in[1] for
	// the handler, and we are about to clear headerSlice[:16] for
	// OutHeader. Reading the (now-zeroed) InHeader through that slice
	// would corrupt the request.
	inIov := [][]byte{
		inBuf[:inHdrLen],
		inBuf[inHdrLen : inHdrLen+opInLen],
	}
	if inSz > 0 {
		inIov = append(inIov, e.payload[:inSz])
	}

	// Carve the reply buffer. Request and reply alias e.payload (the
	// kernel wrote any variable input here, and the kernel will read
	// the reply back from the same region), so we can't hand the
	// handler a slice of e.payload as its op-out slot — it would
	// either see the input still in place, or our pre-clear would
	// destroy the input before the handler reads it. We use a
	// per-entry scratch instead and copy it into e.payload after
	// dispatch.
	//
	// HandleRequest expects out[0] = 16-byte OutHeader slot, out[1] =
	// outSize-byte op-out slot, out[2] = outPayloadSize-byte payload
	// buffer. Sizes are exact — handlers slice req.outPayload down to
	// actual bytes written before HandleRequest returns. Both
	// OutHeader and op-out slots must be zero per the go-fuse API
	// contract (handlers leave untouched fields zero).
	//
	// For ops with variable-length string output (READLINK, ...) the
	// handler reassigns req.outPayload to a freshly-allocated slice
	// rather than writing into our slot. HandleRequest's copy-back
	// step lands the bytes in our buffer iff we passed a non-empty
	// payload slot. So advertise the full payload region for those.
	outHeaderSlot := headerSlice[:sizeOfOutHeader]
	clear(outHeaderSlot)
	outIov := [][]byte{outHeaderSlot}
	if outSize > 0 {
		clear(e.opOutScratch[:outSize])
		outIov = append(outIov, e.opOutScratch[:outSize])
	}
	payloadCap := outPayloadSize
	if h.FileNameOut {
		payloadCap = len(e.payload)
	}
	if payloadCap > 0 {
		outIov = append(outIov, e.payload[:payloadCap])
	}

	n, status := q.ps.handleIov(inIov, outIov)
	if status != OK {
		q.tracef("handleIov entry=%d status=%v", idx, status)
		return q.commitError(idx, status)
	}

	// Land the op-out struct (written into our scratch) at the start
	// of e.payload, where the kernel reads the reply args from. Ops
	// with outSize > 0 always have outPayloadSize == 0 (see
	// parseRequest), so we never have to interleave op-out and bulk
	// data in the same buffer.
	if outSize > 0 {
		copy(e.payload[:outSize], e.opOutScratch[:outSize])
	}

	// Kernel expects payload_sz to count the args bytes only (op-out +
	// trailing data), not the OutHeader.
	payloadSz := n - int(sizeOfOutHeader)
	if payloadSz < 0 {
		payloadSz = 0
	}
	e.header.RingEntInOut.PayloadSz = uint32(payloadSz)

	commitID := e.header.RingEntInOut.CommitID
	return q.submitCommitAndFetch(idx, commitID)
}

// commitError writes a minimal error reply into the entry and submits
// COMMIT_AND_FETCH, so the kernel sees the failure and the slot is reused.
func (q *uringQueue) commitError(idx int, status Status) error {
	e := &q.entries[idx]
	outHdr := (*OutHeader)(unsafe.Pointer(&e.header.InOut[0]))
	outHdr.Length = uint32(sizeOfOutHeader)
	outHdr.Status = -int32(status)
	e.header.RingEntInOut.PayloadSz = 0
	return q.submitCommitAndFetch(idx, e.header.RingEntInOut.CommitID)
}

// Close stops the queue and releases all resources.
func (q *uringQueue) Close() error {
	if q.ring != nil {
		err := q.ring.Close()
		q.ring = nil
		return err
	}
	return nil
}

