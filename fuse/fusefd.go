// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

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

// newFuseFD returns a fuseFD for s, ready to read from fd.
// Ownership of fd passes to the returned fuseFD's *os.File. All shared
// state (pools, buffer pool, accounting constants) is wired up here.
func (s *Server) newFuseFD(fd int, maxReaders int) (*fuseFD, error) {
	file := os.NewFile(uintptr(fd), "/dev/fuse")
	conn, err := file.SyscallConn()
	if err != nil {
		file.Close()
		return nil, err
	}
	_, _, reqAllocBytes := requestAccountingSizes(0)
	r := &fuseFD{
		maxReaders:              maxReaders,
		maxInflightRequestBytes: s.opts.MaxInflightRequestBytes,
		debug:                   s.opts.Debug,
		logger:                  s.opts.Logger,
		file:                    file,
		conn:                    conn,
		reqPool:                 &s.reqPool,
		readPool:                &s.readPool,
		buffers:                 &s.buffers,
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
func (f *fuseFD) stopReading() {
	f.reqMu.Lock()
	f.stopped = true
	f.wake()
	f.reqMu.Unlock()
}

// startReading undoes stopReading.
func (f *fuseFD) startReading() {
	f.reqMu.Lock()
	f.stopped = false
	f.unwake()
	f.reqMu.Unlock()
}

// withFD runs f with the underlying FUSE file descriptor, holding a
// reference on it for the duration so the fd cannot be closed (and its
// number reused) while f runs. This is what lets concurrent writers and
// close() run without a serializing mutex. The error is non-nil only
// when the fd is already closed, in which case fn is not called.
func (f *fuseFD) withFD(fn func(fd int)) error {
	return f.conn.Control(func(fd uintptr) {
		fn(int(fd))
	})
}

// writevFD writes iov to the FUSE fd, holding a reference against
// concurrent close. The error is the underlying syscall error, or a
// non-syscall error if the fd is already closed.
func (f *fuseFD) writevFD(iov [][]byte) (int, error) {
	var n int
	var err error
	if cerr := f.withFD(func(fd int) {
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
func (f *fuseFD) readRequest() (req *requestAlloc, needReader bool, code syscall.Errno) {
	f.reqMu.Lock()
	if f.stopped || f.reqReaders > f.maxReaders || !f.reserveRequestBytes() {
		f.reqMu.Unlock()
		return nil, false, OK
	}
	f.reqReaders++
	f.reqMu.Unlock()

	req = f.reqPool.Get().(*requestAlloc)
	dest := f.readPool.Get().([]byte)

	req.call.fuseFD = f
	n, err := req.call.read(dest)
	if err != nil {
		f.reqMu.Lock()
		f.putReadBuf(dest)
		f.putReq(req)
		f.reqReaders--
		f.reqMu.Unlock()
		if err == errStopped {
			return nil, false, OK
		}
		return nil, false, ToErrno(err)
	}

	f.reqMu.Lock()
	defer f.reqMu.Unlock()
	gobbled := req.setInput(dest[:n])
	if len(req.inputBuf) < int(unsafe.Sizeof(InHeader{})) {
		log.Printf("Short read for input header: %v", req.inputBuf)
		f.putReadBuf(dest)
		f.putReq(req)
		f.reqReaders--
		return nil, false, EINVAL
	}
	opCode := ((*InHeader)(unsafe.Pointer(&req.inputBuf[0]))).Opcode
	/* These messages don't expect reply, so they cost nothing for
	   the kernel to send. Make sure we're not overwhelmed by not
	   spawning a new reader.
	*/
	needsBackPressure := (opCode == _OP_FORGET || opCode == _OP_BATCH_FORGET)

	if !gobbled {
		f.putReadBuf(dest)
	}
	f.reqReaders--
	return req, f.reqReaders <= 0 && !needsBackPressure, OK
}

// returnRequest returns a request to the pool of unused requests.
func (f *fuseFD) returnRequest(req *requestAlloc) {
	if req.bufferPoolOutputBuf != nil {
		f.buffers.FreeBuffer(req.bufferPoolOutputBuf)
		req.bufferPoolOutputBuf = nil
	}
	req.clear()

	f.reqMu.Lock()
	if p := req.bufferPoolInputBuf; p != nil {
		req.bufferPoolInputBuf = nil
		f.putReadBuf(p)
	}
	f.putReq(req)
	f.reqMu.Unlock()
}

func (f *fuseFD) reserveRequestBytes() bool {
	if !f.canReserveRequestBytes() {
		return false
	}
	f.inflightRequestBytes += f.requestBytes()
	return true
}

func (f *fuseFD) canReserveRequestBytes() bool {
	return f.inflightRequestBytes == 0 ||
		f.requestBytes() <= f.maxInflightRequestBytes-f.inflightRequestBytes
}

// canAcceptAnother wraps canReserveRequestBytes with reqMu, for callers
// that don't already hold the lock.
func (f *fuseFD) canAcceptAnother() bool {
	f.reqMu.Lock()
	defer f.reqMu.Unlock()
	return f.canReserveRequestBytes()
}

func (f *fuseFD) requestBytes() int {
	return f.reqAllocBytes + f.readBufBytes
}

func (f *fuseFD) putReadBuf(buf []byte) {
	f.readPool.Put(buf)
	f.inflightRequestBytes -= f.readBufBytes
}

func (f *fuseFD) putReq(req *requestAlloc) {
	f.reqPool.Put(req)
	f.inflightRequestBytes -= f.reqAllocBytes
}

// close closes the underlying FUSE fd. The *os.File waits for in-flight
// RawConn operations (reads, writes, ioctls) before releasing the fd.
// Readers must have exited.
func (f *fuseFD) close() error {
	err := f.file.Close()
	f.reqMu.Lock()
	if f.wakeFD >= 0 {
		syscall.Close(f.wakeFD)
		f.wakeFD = -1
	}
	for _, efd := range f.epollFDs {
		syscall.Close(efd)
	}
	f.epollFDs = nil
	f.reqMu.Unlock()
	return err
}

func (f *fuseFD) writev(iov [][]byte) (int, syscall.Errno) {
	n, err := f.writevFD(iov)
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

func (f *fuseFD) writeWithSlice(req *request, ws withSlice) syscall.Errno {
	slices, st := ws.Slices()
	if st != OK {
		return st
	}
	bufs := make([][]byte, 0, 2+len(slices))
	bufs = append(bufs, req.outHeaderBuf, req.outDataBuf)
	bufs = append(bufs, slices...)
	_, err := f.writevFD(bufs)
	return ToErrno(err)
}
