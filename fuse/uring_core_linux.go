// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

// Minimal io_uring runtime, hand-rolled because golang.org/x/sys/unix at
// the pinned version does not expose the syscalls. This file is the
// transport-agnostic half: syscalls, SQE/CQE layouts, and a ring struct
// that owns the mmap'd SQ/CQ regions. FUSE-specific code lives next door
// in uring_dispatch_linux.go.
//
// Source of truth: include/uapi/linux/io_uring.h, Linux ≥6.10.

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// io_uring syscall numbers (x86_64; same on arm64 in the relevant range).
const (
	_SYS_IO_URING_SETUP    = 425
	_SYS_IO_URING_ENTER    = 426
	_SYS_IO_URING_REGISTER = 427
)

// Setup flags. Only the ones we use are named.
const (
	_IORING_SETUP_SQE128       = 1 << 10
	_IORING_SETUP_CQE32        = 1 << 11
	_IORING_SETUP_SINGLE_ISSUER = 1 << 12
	_IORING_SETUP_DEFER_TASKRUN = 1 << 13
)

// Generic SQE opcodes (only the ones we use).
const (
	_IORING_OP_NOP       = 0
	_IORING_OP_URING_CMD = 46
)

// io_uring_enter flags.
const (
	_IORING_ENTER_GETEVENTS = 1 << 0
)

// io_uring_sqe is 64 bytes when IORING_SETUP_SQE128 is off, 128 bytes when
// it is on (the trailing 64 bytes extend the cmd[] payload). FUSE-over-
// io_uring needs SQE128 because struct fuse_uring_cmd_req exceeds the
// 16-byte cmd[] of a stock SQE.
//
// We define the first 64 bytes as named fields plus a cmd[80] tail that
// covers both the stock 16-byte cmd[] and the SQE128 extension.
type ioUringSqe struct {
	Opcode      uint8
	Flags       uint8
	IoPrio      uint16
	Fd          int32
	Off         uint64 // also: cmd_op (low 32 bits) for URING_CMD
	Addr        uint64
	Len         uint32
	OpFlags     uint32
	UserData    uint64
	BufIndex    uint16
	Personality uint16
	SpliceFdIn  int32
	Addr3       uint64
	_           [1]uint64 // __pad2
	// cmd[80] starts at offset 48 for URING_CMD with SQE128. The first
	// 16 bytes overlay Addr3+pad2; the next 64 bytes are the SQE128
	// extension. We address them via CmdPayload() below.
	CmdExt [64]byte
}

func (s *ioUringSqe) CmdPayload() []byte {
	// cmd[80] = 16 bytes (Addr3 + pad2) + 64 bytes (CmdExt)
	return unsafe.Slice((*byte)(unsafe.Pointer(&s.Addr3)), 80)
}

// ioUringCqe — stock 16-byte completion entry. (We do not request CQE32.)
type ioUringCqe struct {
	UserData uint64
	Res      int32
	Flags    uint32
}

// ioSqringOffsets / ioCqringOffsets describe the layout of the mmap'd
// rings.  Mirrors struct io_sqring_offsets / io_cqring_offsets.
type ioSqringOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Flags       uint32
	Dropped     uint32
	Array       uint32
	Resv1       uint32
	UserAddr    uint64
}

type ioCqringOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Overflow    uint32
	Cqes        uint32
	Flags       uint32
	Resv1       uint32
	UserAddr    uint64
}

// ioUringParams mirrors struct io_uring_params.
type ioUringParams struct {
	SqEntries    uint32
	CqEntries    uint32
	Flags        uint32
	SqThreadCpu  uint32
	SqThreadIdle uint32
	Features     uint32
	WqFd         uint32
	Resv         [3]uint32
	SqOff        ioSqringOffsets
	CqOff        ioCqringOffsets
}

// Static size assertions.
const (
	_ uint = uint(unsafe.Sizeof(ioUringSqe{})) - 128
	_ uint = 128 - uint(unsafe.Sizeof(ioUringSqe{}))

	_ uint = uint(unsafe.Sizeof(ioUringCqe{})) - 16
	_ uint = 16 - uint(unsafe.Sizeof(ioUringCqe{}))

	_ uint = uint(unsafe.Sizeof(ioUringParams{})) - 120
	_ uint = 120 - uint(unsafe.Sizeof(ioUringParams{}))
)

func ioUringSetup(entries uint32, p *ioUringParams) (int, error) {
	r1, _, errno := syscall.Syscall(_SYS_IO_URING_SETUP,
		uintptr(entries), uintptr(unsafe.Pointer(p)), 0)
	if errno != 0 {
		return -1, errno
	}
	return int(r1), nil
}

func ioUringEnter(fd int, toSubmit, minComplete uint32, flags uint32) (int, error) {
	r1, _, errno := syscall.Syscall6(_SYS_IO_URING_ENTER,
		uintptr(fd), uintptr(toSubmit), uintptr(minComplete),
		uintptr(flags), 0, 0)
	if errno != 0 {
		return -1, errno
	}
	return int(r1), nil
}

func ioUringRegister(fd int, op uint32, arg unsafe.Pointer, nrArgs uint32) (int, error) {
	r1, _, errno := syscall.Syscall6(_SYS_IO_URING_REGISTER,
		uintptr(fd), uintptr(op), uintptr(arg), uintptr(nrArgs), 0, 0)
	if errno != 0 {
		return -1, errno
	}
	return int(r1), nil
}

// uringRing owns one io_uring instance: the ring fd plus the SQ and CQ
// mmap regions, with cached pointers into the head/tail/array fields.
type uringRing struct {
	fd      int
	entries uint32

	sqMmap []byte
	cqMmap []byte // may alias sqMmap on IORING_FEAT_SINGLE_MMAP kernels

	sqeMmap []byte
	sqes    []ioUringSqe

	sqHead    *uint32
	sqTail    *uint32
	sqMask    uint32
	sqArray   []uint32

	cqHead *uint32
	cqTail *uint32
	cqMask uint32
	cqes   []ioUringCqe
}

func newUringRing(entries uint32, flags uint32) (*uringRing, error) {
	var p ioUringParams
	p.Flags = flags

	fd, err := ioUringSetup(entries, &p)
	if err != nil {
		return nil, fmt.Errorf("io_uring_setup: %w", err)
	}
	r := &uringRing{fd: fd, entries: p.SqEntries}

	sqSize := uintptr(p.SqOff.Array) + uintptr(p.SqEntries)*unsafe.Sizeof(uint32(0))
	cqSize := uintptr(p.CqOff.Cqes) + uintptr(p.CqEntries)*unsafe.Sizeof(ioUringCqe{})

	const (
		offSqRing = 0
		offCqRing = 0x8000000
		offSqes   = 0x10000000
	)

	sq, err := syscall.Mmap(fd, offSqRing, int(sqSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("mmap sq: %w", err)
	}
	r.sqMmap = sq

	cq, err := syscall.Mmap(fd, offCqRing, int(cqSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("mmap cq: %w", err)
	}
	r.cqMmap = cq

	sqeBytes := int(p.SqEntries) * int(unsafe.Sizeof(ioUringSqe{}))
	sqe, err := syscall.Mmap(fd, offSqes, sqeBytes,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("mmap sqes: %w", err)
	}
	r.sqeMmap = sqe
	r.sqes = unsafe.Slice((*ioUringSqe)(unsafe.Pointer(&sqe[0])), p.SqEntries)

	r.sqHead = (*uint32)(unsafe.Pointer(&sq[p.SqOff.Head]))
	r.sqTail = (*uint32)(unsafe.Pointer(&sq[p.SqOff.Tail]))
	r.sqMask = *(*uint32)(unsafe.Pointer(&sq[p.SqOff.RingMask]))
	r.sqArray = unsafe.Slice(
		(*uint32)(unsafe.Pointer(&sq[p.SqOff.Array])), p.SqEntries)

	r.cqHead = (*uint32)(unsafe.Pointer(&cq[p.CqOff.Head]))
	r.cqTail = (*uint32)(unsafe.Pointer(&cq[p.CqOff.Tail]))
	r.cqMask = *(*uint32)(unsafe.Pointer(&cq[p.CqOff.RingMask]))
	r.cqes = unsafe.Slice(
		(*ioUringCqe)(unsafe.Pointer(&cq[p.CqOff.Cqes])), p.CqEntries)

	// Pre-init the SQ array as identity: sqArray[i] = i. This way each
	// SQ slot directly references the SQE at the same index.
	for i := range r.sqArray {
		r.sqArray[i] = uint32(i)
	}

	return r, nil
}

// nextSqe reserves the next SQE slot. Returns nil when the SQ is full
// (caller should ioUringEnter and retry).
func (r *uringRing) nextSqe() *ioUringSqe {
	tail := atomic.LoadUint32(r.sqTail)
	head := atomic.LoadUint32(r.sqHead)
	if tail-head >= r.entries {
		return nil
	}
	sqe := &r.sqes[tail&r.sqMask]
	*sqe = ioUringSqe{} // zero the slot
	return sqe
}

// commitSqe advances the SQ tail by one. Caller must have populated the
// SQE returned by nextSqe().
func (r *uringRing) commitSqe() {
	atomic.StoreUint32(r.sqTail, atomic.LoadUint32(r.sqTail)+1)
}

// peekCqe returns the next unconsumed CQE, or nil if the CQ is empty.
func (r *uringRing) peekCqe() *ioUringCqe {
	head := atomic.LoadUint32(r.cqHead)
	tail := atomic.LoadUint32(r.cqTail)
	if head == tail {
		return nil
	}
	return &r.cqes[head&r.cqMask]
}

// advanceCq marks the CQE returned by peekCqe as consumed.
func (r *uringRing) advanceCq() {
	atomic.StoreUint32(r.cqHead, atomic.LoadUint32(r.cqHead)+1)
}

// Close releases all mmap regions and the ring fd.
func (r *uringRing) Close() error {
	if r.sqeMmap != nil {
		syscall.Munmap(r.sqeMmap)
		r.sqeMmap = nil
	}
	if r.cqMmap != nil && len(r.cqMmap) > 0 {
		syscall.Munmap(r.cqMmap)
	}
	r.cqMmap = nil
	if r.sqMmap != nil {
		syscall.Munmap(r.sqMmap)
		r.sqMmap = nil
	}
	if r.fd >= 0 {
		err := syscall.Close(r.fd)
		r.fd = -1
		return err
	}
	return nil
}
