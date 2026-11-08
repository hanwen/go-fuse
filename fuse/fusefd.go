// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"errors"
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// fuseFD owns the per-FUSE-fd state: the fd itself and the read-side
// bookkeeping (reqReaders, inflightRequestBytes). The pools are shared
// with the owning Server via pointers, and settings are copied from it.
//
// Today a Server has a single fuseFD; this struct exists so that adding
// support for FUSE_DEV_IOC_CLONE'd fds is a matter of widening the field
// to a slice.
type fuseFD struct {
	maxReaders              int
	maxInflightRequestBytes int
	debug                   bool
	logger                  *log.Logger

	// canSplice is set by Server.setSplice, after INIT.
	canSplice bool

	// I/O with kernel and daemon. file owns the FUSE fd; conn is its
	// RawConn. Running syscalls through conn holds a reference on the
	// fd, so concurrent writers and close() are safe against the fd
	// number being reused without any explicit locking.
	file *os.File
	conn syscall.RawConn

	reqMu                sync.Mutex
	reqReaders           int
	inflightRequestBytes int

	// stopped is set by stopReading. Protected by reqMu.
	stopped bool

	// epollFDs are epoll fds that no reader is waiting in.
	// Protected by reqMu.
	epollFDs []int

	// wakeFD becomes readable on stopReading, and wakes readers
	// waiting for the FUSE fd. -1 if reads cannot be interrupted, or
	// after close. Changes under reqMu.
	wakeFD int

	// Shared pools owned by Server.
	reqPool  *sync.Pool
	readPool *sync.Pool
	buffers  *bufferPool

	// Accounting constants, set by Server.setMaxWrite.
	reqAllocBytes int
	readBufBytes  int
}

// newFuseFD returns a fuseFD for ms, ready to read from fd.
// Ownership of fd passes to the returned fuseFD's *os.File. All shared
// state (pools, buffer pool, accounting constants) is wired up here.
func (ms *Server) newFuseFD(fd int, maxReaders int) (*fuseFD, error) {
	file := os.NewFile(uintptr(fd), "/dev/fuse")
	conn, err := file.SyscallConn()
	if err != nil {
		file.Close()
		return nil, err
	}
	_, _, reqAllocBytes := requestAccountingSizes(0)
	r := &fuseFD{
		maxReaders:              maxReaders,
		maxInflightRequestBytes: ms.opts.MaxInflightRequestBytes,
		debug:                   ms.opts.Debug,
		logger:                  ms.opts.Logger,
		file:                    file,
		conn:                    conn,
		reqPool:                 &ms.reqPool,
		readPool:                &ms.readPool,
		buffers:                 &ms.buffers,
		reqAllocBytes:           reqAllocBytes,
		wakeFD:                  -1,
	}
	// After os.NewFile, so the runtime poller does not pick up the
	// fd: readers poll it themselves.
	if err := r.initWake(fd); err != nil {
		file.Close()
		return nil, err
	}
	return r, nil
}

// errStopped is returned by readFD when stopReading interrupted it.
var errStopped = errors.New("fuse: reading stopped")

// stopReading makes readers return without a request, including
// readers waiting for one. On platforms where reads cannot be
// interrupted, a reader blocked in read(2) returns with the next
// request.
func (r *fuseFD) stopReading() {
	r.reqMu.Lock()
	r.stopped = true
	r.wake()
	r.reqMu.Unlock()
}

// withFD runs f with the underlying FUSE file descriptor, holding a
// reference on it for the duration so the fd cannot be closed (and its
// number reused) while f runs. This is what lets concurrent writers and
// close() run without a serializing mutex. The error is non-nil only
// when the fd is already closed, in which case f is not called.
func (r *fuseFD) withFD(f func(fd int)) error {
	return r.conn.Control(func(fd uintptr) {
		f(int(fd))
	})
}

// writevFD writes iov to the FUSE fd, holding a reference against
// concurrent close. The error is the underlying syscall error, or a
// non-syscall error if the fd is already closed.
func (r *fuseFD) writevFD(iov [][]byte) (int, error) {
	var n int
	var err error
	if cerr := r.withFD(func(fd int) {
		n, err = writev(fd, iov)
	}); cerr != nil {
		return 0, cerr
	}
	return n, err
}

// fdCall is a read or write on the FUSE fd. It is part of the pooled
// requestAlloc, so the functions passed to RawConn.Control are
// allocated once, rather than a closure for every call.
type fdCall struct {
	fuseFD *fuseFD

	buf       []byte
	iov       [][]byte
	iovInline [3][]byte
	n         int
	err       error

	readFn  func(fd uintptr)
	writeFn func(fd uintptr)
}

func (c *fdCall) doRead(fd uintptr) {
	c.n, c.err = c.fuseFD.readFD(int(fd), c.buf)
}

func (c *fdCall) doWrite(fd uintptr) {
	c.n, c.err = writev(int(fd), c.iov)
}

// read reads into buf, retrying on EINTR.
func (c *fdCall) read(buf []byte) (int, error) {
	if c.readFn == nil {
		c.readFn = c.doRead
	}
	c.buf = buf
	for {
		if err := c.fuseFD.conn.Control(c.readFn); err != nil {
			c.err = err
			break
		}
		if c.err != syscall.EINTR {
			break
		}
	}
	n, err := c.n, c.err
	c.buf, c.n, c.err = nil, 0, nil
	return n, err
}

// writev writes bufs, retrying on EINTR.
func (c *fdCall) writev(bufs ...[]byte) (int, error) {
	if c.writeFn == nil {
		c.writeFn = c.doWrite
	}
	c.iov = append(c.iovInline[:0], bufs...)
	for {
		if err := c.fuseFD.conn.Control(c.writeFn); err != nil {
			c.err = err
			break
		}
		if c.err != syscall.EINTR {
			break
		}
	}
	n, err := c.n, c.err
	clear(c.iovInline[:])
	c.iov, c.n, c.err = nil, 0, nil
	return n, err
}

// readRequest reads one request from the kernel. Returns nil, OK if
// reading was stopped, there are too many concurrent readers or
// insufficient request-bytes budget. needReader is set if no other
// reader is waiting for a request, and the request is one that the
// kernel expects a reply for.
func (r *fuseFD) readRequest() (req *requestAlloc, needReader bool, code Status) {
	r.reqMu.Lock()
	if r.stopped || r.reqReaders > r.maxReaders || !r.reserveRequestBytes() {
		r.reqMu.Unlock()
		return nil, false, OK
	}
	r.reqReaders++
	r.reqMu.Unlock()

	req = r.reqPool.Get().(*requestAlloc)
	dest := r.readPool.Get().([]byte)

	req.call.fuseFD = r
	n, err := req.call.read(dest)
	if err != nil {
		r.reqMu.Lock()
		r.putReadBuf(dest)
		r.putReq(req)
		r.reqReaders--
		r.reqMu.Unlock()
		if err == errStopped {
			return nil, false, OK
		}
		return nil, false, ToStatus(err)
	}

	r.reqMu.Lock()
	defer r.reqMu.Unlock()
	gobbled := req.setInput(dest[:n])
	if len(req.inputBuf) < int(unsafe.Sizeof(InHeader{})) {
		log.Printf("Short read for input header: %v", req.inputBuf)
		r.putReadBuf(dest)
		r.putReq(req)
		r.reqReaders--
		return nil, false, EINVAL
	}
	opCode := ((*InHeader)(unsafe.Pointer(&req.inputBuf[0]))).Opcode
	/* These messages don't expect reply, so they cost nothing for
	   the kernel to send. Make sure we're not overwhelmed by not
	   spawning a new reader.
	*/
	needsBackPressure := (opCode == _OP_FORGET || opCode == _OP_BATCH_FORGET)

	if !gobbled {
		r.putReadBuf(dest)
	}
	r.reqReaders--
	return req, r.reqReaders <= 0 && !needsBackPressure, OK
}

// returnRequest returns a request to the pool of unused requests.
func (r *fuseFD) returnRequest(req *requestAlloc) {
	if req.bufferPoolOutputBuf != nil {
		r.buffers.FreeBuffer(req.bufferPoolOutputBuf)
		req.bufferPoolOutputBuf = nil
	}
	req.clear()

	r.reqMu.Lock()
	if p := req.bufferPoolInputBuf; p != nil {
		req.bufferPoolInputBuf = nil
		r.putReadBuf(p)
	}
	r.putReq(req)
	r.reqMu.Unlock()
}

func (r *fuseFD) reserveRequestBytes() bool {
	if !r.canReserveRequestBytes() {
		return false
	}
	r.inflightRequestBytes += r.requestBytes()
	return true
}

func (r *fuseFD) canReserveRequestBytes() bool {
	return r.inflightRequestBytes == 0 ||
		r.requestBytes() <= r.maxInflightRequestBytes-r.inflightRequestBytes
}

// canAcceptAnother wraps canReserveRequestBytes with reqMu, for callers
// that don't already hold the lock.
func (r *fuseFD) canAcceptAnother() bool {
	r.reqMu.Lock()
	defer r.reqMu.Unlock()
	return r.canReserveRequestBytes()
}

func (r *fuseFD) requestBytes() int {
	return r.reqAllocBytes + r.readBufBytes
}

func (r *fuseFD) putReadBuf(buf []byte) {
	r.readPool.Put(buf)
	r.inflightRequestBytes -= r.readBufBytes
}

func (r *fuseFD) putReq(req *requestAlloc) {
	r.reqPool.Put(req)
	r.inflightRequestBytes -= r.reqAllocBytes
}

// close closes the underlying FUSE fd. The *os.File waits for in-flight
// RawConn operations (reads, writes, ioctls) before releasing the fd.
// Readers must have exited.
func (r *fuseFD) close() error {
	err := r.file.Close()
	r.reqMu.Lock()
	if r.wakeFD >= 0 {
		syscall.Close(r.wakeFD)
		r.wakeFD = -1
	}
	for _, efd := range r.epollFDs {
		syscall.Close(efd)
	}
	r.epollFDs = nil
	r.reqMu.Unlock()
	return err
}

func (r *fuseFD) writev(iov [][]byte) (int, syscall.Errno) {
	n, err := r.writevFD(iov)
	if err == nil {
		return n, 0
	}
	errno, ok := err.(syscall.Errno)
	if !ok {
		// The fd is closed; report it as such.
		return n, syscall.EBADF
	}
	if errno == syscall.EINVAL {
		// Detail: the kernel returns EINVAL for unsupported
		// notify methods.
		errno = syscall.ENOSYS
	}
	return n, errno
}

func (r *fuseFD) writeWithSlice(req *request, ws withSlice) Status {
	slices, st := ws.Slices()
	if !st.Ok() {
		return st
	}
	bufs := make([][]byte, 0, 2+len(slices))
	bufs = append(bufs, req.outHeaderBuf, req.outDataBuf)
	bufs = append(bufs, slices...)
	_, err := r.writevFD(bufs)
	return ToStatus(err)
}
