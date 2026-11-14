// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	// Linux v4.20+ caps requests at 1 MiB. Older kernels at 128
	// kiB. Deprecated: current linux kernels allow tuning this
	// using sysctl.
	MAX_KERNEL_WRITE = 1024 * 1024

	// Linux kernel constant from include/uapi/linux/fuse.h
	// Reads from /dev/fuse that are smaller fail with EINVAL.
	_FUSE_MIN_READ_BUFFER = 8192

	minMaxReaders = 2
	maxMaxReaders = 16
)

// Server contains the logic for reading from the FUSE device.
type Server struct {
	protocolServer

	// Empty if unmounted.
	mount *Mount

	fuseFD *fuseFD

	opts *MountOptions

	// Pool for request structs.
	reqPool sync.Pool

	// Pool for raw requests data
	readPool sync.Pool

	singleReader bool
	serving      atomic.Bool // for preventing duplicate Serve() calls

	// lifeMu serializes Start, Stop and Shutdown.
	lifeMu sync.Mutex

	// stateMu protects state, loops, loopsZero and dead.
	stateMu sync.Mutex
	state   serverState

	// loops counts goroutines reading or serving requests. loopsZero
	// is closed when it drops to zero.
	loops     int
	loopsZero chan struct{}

	// dead is set when reading failed, eg. after unmount.
	dead bool

	// done is closed when the server is closed.
	done chan struct{}

	// notifyStopped makes notifications fail with EINTR. Kernel
	// operations that a notification waits for may be blocked by a
	// request that nobody reads.
	notifyStopped atomic.Bool

	// Used to implement WaitMount on macos.
	ready <-chan error

	// for implementing single threaded processing.
	requestProcessingMu sync.Mutex
}

// KernelSettings returns the Init message from the kernel, so
// filesystems can adapt to availability of features of the kernel
// driver. The message should not be altered.
func (s *Server) KernelSettings() *InitIn {
	in := s.conn.in

	return &in
}

// Unmount unmounts the file system (see Mount.Unmount), and
// waits for the server to exit. After the Server is unmounted, it
// should be discarded. This function is idempotent.
func (s *Server) Unmount() error {
	if s.mount == nil {
		return errors.New("fuse: server has no mount")
	}
	if err := s.mount.Unmount(); err != nil {
		return err
	}
	// The kernel has aborted the connection.
	s.cancelAll()
	s.markDead()
	<-s.done
	return nil
}

// alignSlice ensures that the byte at alignedByte is aligned with the
// given logical block size.  The input slice should be at least (size
// + blockSize)
func alignSlice(buf []byte, alignedByte, blockSize, size uintptr) []byte {
	misaligned := uintptr(unsafe.Pointer(&buf[alignedByte])) & (blockSize - 1)
	buf = buf[blockSize-misaligned:]
	return buf[:size]
}

func defaultMountOptions(opts *MountOptions) MountOptions {
	if opts == nil {
		return MountOptions{
			MaxBackground: _DEFAULT_BACKGROUND_TASKS,
		}
	}
	return *opts
}

// NewServer creates a FUSE server and attaches ("mounts") it to the
// `mountPoint` directory. It is NewMount and Mount.Mount, followed by
// NewServerFromDev.
//
// See the "Mount styles" section in the package documentation if you want to
// know about the inner workings of the mount process. Usually you do not.
func NewServer(fs RawFileSystem, mountPoint string, opts *MountOptions) (*Server, error) {
	o := defaultMountOptions(opts)
	o.setDefaults(fs)
	m, err := NewMount(mountPoint)
	if err != nil {
		return nil, err
	}
	d, err := m.Mount(&o)
	if err != nil {
		return nil, err
	}
	return newServerFromDev(fs, d, m, &o)
}

// NewServerFromDev creates a server for the FUSE device d. If the
// connection was not negotiated yet, it reads INIT from the kernel, and
// calls RawFileSystem.Init, MountOptions.InitHandler and OnConnect.
// For a device from Server.Dev, it uses the connection as negotiated
// before, and only calls OnConnect. It fails if this version of
// go-fuse cannot serve that connection. MountOptions only affect
// local behavior then. The fields of opts that only apply to mounting
// are always ignored.
//
// m is where d is mounted, for Unmount and WaitMount. It may be nil;
// Unmount then fails.
//
// The server is returned stopped; Start or Serve starts it.
// NewServerFromDev takes ownership of d, also if it fails.
func NewServerFromDev(fs RawFileSystem, d *Dev, m *Mount, opts *MountOptions) (*Server, error) {
	o := defaultMountOptions(opts)
	o.setDefaults(fs)
	return newServerFromDev(fs, d, m, &o)
}

func newServerFromDev(fs RawFileSystem, d *Dev, m *Mount, o *MountOptions) (*Server, error) {
	maxReaders := runtime.GOMAXPROCS(0)
	if maxReaders < minMaxReaders {
		maxReaders = minMaxReaders
	} else if maxReaders > maxMaxReaders {
		maxReaders = maxMaxReaders
	}

	ms := &Server{
		protocolServer: protocolServer{
			fileSystem: fs,
			opts:       o,
			conn:       &Conn{},
		},
		opts:         o,
		mount:        m,
		singleReader: useSingleReader,
		ready:        d.ready,
		done:         make(chan struct{}),
	}
	ms.reqPool.New = func() any {
		return &requestAlloc{}
	}
	if d.negotiated() {
		if err := checkSession(&d.in, &d.out); err != nil {
			d.Close()
			return nil, err
		}
	}
	var err error
	ms.fuseFD, err = ms.newFuseFD(d.release(), maxReaders)
	if err != nil {
		return nil, err
	}
	ms.setMaxWrite(o.MaxWrite)
	ms.conn.notify = newNotify(ms.writeNotify, o)
	ms.conn.backing = ms.fdBacking()

	if d.negotiated() {
		ms.connect(&d.in, &d.out)
		ms.applyNegotiated()
	} else if code := ms.handleInit(); !code.Ok() {
		ms.fuseFD.close()
		// TODO - unmount as well?
		return nil, fmt.Errorf("init: %s", code)
	}
	return ms, nil
}

// setMaxWrite sizes the read buffers for requests carrying up to
// maxWrite bytes of data. It must be called while no request is read.
func (s *Server) setMaxWrite(maxWrite int) {
	readBufSize, readBufBytes, _ := requestAccountingSizes(maxWrite)
	s.readPool = sync.Pool{
		New: func() any {
			// O_DIRECT typically requires buffers aligned to
			// blocksize (see man 2 open), but requirements vary
			// across file systems. Presumably, we could also fix
			// this by reading the requests using readv.
			buf := make([]byte, readBufBytes)
			buf = alignSlice(buf, unsafe.Sizeof(WriteIn{}), logicalBlockSize, uintptr(readBufSize))
			return buf
		},
	}
	s.fuseFD.readBufBytes = readBufBytes
}

// applyNegotiated sizes the read buffers and enables splice for
// the negotiated connection.
func (s *Server) applyNegotiated() {
	s.setMaxWrite(int(s.conn.out.MaxWrite))
	if s.conn.out.Minor >= 13 {
		s.setSplice()
	}
}

func requestAccountingSizes(maxWrite int) (readBufSize, readBufBytes, reqAllocBytes int) {
	readBufSize = max(maxWrite+int(maxInputSize), _FUSE_MIN_READ_BUFFER)
	readBufBytes = readBufSize + logicalBlockSize
	reqAllocBytes = int(unsafe.Sizeof(requestAlloc{}))
	return
}

func escape(optionValue string) string {
	return strings.Replace(strings.Replace(optionValue, `\`, `\\`, -1), `,`, `\,`, -1)
}

func (o *MountOptions) optionsStrings() []string {
	var r []string
	r = append(r, o.Options...)

	if o.AllowOther {
		r = append(r, "allow_other")
	}
	if o.FsName != "" {
		r = append(r, "fsname="+o.FsName)
	}
	if o.Name != "" {
		r = append(r, "subtype="+o.Name)
	}
	r = append(r, fmt.Sprintf("max_read=%d", o.MaxWrite))

	// OSXFUSE applies a 60-second timeout for file operations. This
	// is inconsistent with how FUSE works on Linux, where operations
	// last as long as the daemon is willing to let them run.
	if runtime.GOOS == "darwin" {
		r = append(r, "daemon_timeout=0")
	}
	if o.IDMappedMount && !o.containsOption("default_permissions") {
		r = append(r, "default_permissions")
	}

	// Commas and backslashs in an option need to be escaped, because
	// options are separated by a comma and backslashs are used to
	// escape other characters.
	var rEscaped []string
	for _, s := range r {
		rEscaped = append(rEscaped, escape(s))
	}

	return rEscaped
}

func (o *MountOptions) containsOption(opt string) bool {
	return slices.Contains(o.Options, opt)
}

// DebugData returns internal status information for debugging
// purposes.
func (s *Server) DebugData() string {
	var r int
	s.fuseFD.reqMu.Lock()
	r = s.fuseFD.reqReaders
	s.fuseFD.reqMu.Unlock()

	return fmt.Sprintf("readers: %d", r)
}

// handleEINTR retries the given function until it doesn't return syscall.EINTR.
// This is similar to the HANDLE_EINTR() macro from Chromium ( see
// https://code.google.com/p/chromium/codesearch#chromium/src/base/posix/eintr_wrapper.h
// ) and the TEMP_FAILURE_RETRY() from glibc (see
// https://www.gnu.org/software/libc/manual/html_node/Interrupted-Primitives.html
// ).
//
// Don't use handleEINTR() with syscall.Close(); see
// https://code.google.com/p/chromium/issues/detail?id=269623 .
func handleEINTR(fn func() error) (err error) {
	for {
		err = fn()
		if err != syscall.EINTR {
			break
		}
	}
	return
}

// Serve starts serving requests, and waits until the server is
// closed: when the file system is unmounted, or by Shutdown. Normally,
// callers should run Serve() and wait for it to exit, but tests will
// want to run this in a goroutine.
//
// Each filesystem operation executes in a separate goroutine.
func (s *Server) Serve() {
	if !s.serving.CompareAndSwap(false, true) {
		// Calling Serve() multiple times leads to a panic on unmount and fun
		// debugging sessions ( https://github.com/hanwen/go-fuse/issues/512 ).
		// Catch it early.
		log.Panic("Serve() must only be called once, you have called it a second time")
	}
	if err := s.Start(); err != nil {
		s.opts.Logger.Printf("Serve: %v", err)
	}
	s.Wait()
}

// Wait waits until the server is closed.
func (s *Server) Wait() {
	<-s.done
}

type serverState int

const (
	stateStopped serverState = iota
	stateRunning
	stateClosing
	stateClosed
)

var errServerClosed = errors.New("fuse: server closed")

// Start starts reading requests, or resumes after Stop.
func (s *Server) Start() error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	switch s.state {
	case stateRunning:
		return nil
	case stateClosing, stateClosed:
		return errServerClosed
	}
	if s.dead {
		return errServerClosed
	}
	s.fuseFD.startReading()
	s.state = stateRunning
	s.notifyStopped.Store(false)
	s.loops++
	go s.loop()
	return nil
}

// Stop stops reading requests, and waits until every request that
// was read has been answered. The connection stays open, so the
// kernel queues new requests until Start is called, or another
// server takes over the connection. While stopped, notifications
// return EINTR.
//
// Before reading stops, Stop waits for pending NOTIFY_RETRIEVE
// replies. If ctx expires, the server stays stopped, and Stop
// returns the context's error.
//
// On platforms other than Linux, a reader blocked in read(2) only
// returns with the next request, so Stop may wait for it.
func (s *Server) Stop(ctx context.Context) error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	return s.stop(ctx)
}

func (s *Server) stop(ctx context.Context) error {
	s.stateMu.Lock()
	switch s.state {
	case stateClosing, stateClosed:
		s.stateMu.Unlock()
		return errServerClosed
	}
	s.state = stateStopped
	s.notifyStopped.Store(true)
	s.stateMu.Unlock()

	var err error
	if n := s.conn.notify.notifier; n != nil {
		err = n.waitRetrieves(ctx)
	}
	s.fuseFD.stopReading()
	if err == nil {
		err = s.waitLoopsZero(ctx)
	}
	return err
}

// Shutdown stops the server as Stop does, and then closes it: it
// closes this server's file descriptor, and calls
// RawFileSystem.OnUnmount. It does not unmount the file system; if
// no other process has the FUSE device open, the kernel aborts the
// connection. If Stop fails, the server stays stopped.
func (s *Server) Shutdown(ctx context.Context) error {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	if err := s.stop(ctx); err != nil {
		if err == errServerClosed {
			<-s.done
			return nil
		}
		return err
	}
	s.finish()
	return nil
}

// addLoop counts a goroutine that reads or serves requests. The
// caller must be counted itself, so the count is not zero.
func (s *Server) addLoop() {
	s.stateMu.Lock()
	s.loops++
	s.stateMu.Unlock()
}

// loopDone is called when a goroutine counted by addLoop exits.
func (s *Server) loopDone() {
	s.stateMu.Lock()
	s.loops--
	finish := s.checkLoopsZeroLocked()
	s.stateMu.Unlock()
	if finish {
		s.finish()
	}
}

// markDead marks reading as failed for good, so readers exit, and
// the server closes once they have.
func (s *Server) markDead() {
	s.stateMu.Lock()
	s.dead = true
	s.fuseFD.stopReading()
	finish := s.checkLoopsZeroLocked()
	s.stateMu.Unlock()
	if finish {
		s.finish()
	}
}

// checkLoopsZeroLocked closes loopsZero if no goroutine reads or serves
// requests. It returns whether the server should then be closed,
// because reading has failed.
func (s *Server) checkLoopsZeroLocked() bool {
	if s.loops > 0 {
		return false
	}
	if s.loopsZero != nil {
		close(s.loopsZero)
		s.loopsZero = nil
	}
	return s.dead
}

// waitLoopsZero waits until no goroutine reads or serves requests.
func (s *Server) waitLoopsZero(ctx context.Context) error {
	s.stateMu.Lock()
	if s.loops == 0 {
		s.stateMu.Unlock()
		return nil
	}
	if s.loopsZero == nil {
		s.loopsZero = make(chan struct{})
	}
	ch := s.loopsZero
	s.stateMu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// finish closes the server once. It must be called with no request
// being read or served.
func (s *Server) finish() {
	s.stateMu.Lock()
	if s.state == stateClosing || s.state == stateClosed {
		s.stateMu.Unlock()
		<-s.done
		return
	}
	s.state = stateClosing
	s.stateMu.Unlock()

	s.fuseFD.close()
	// shutdown in-flight cache retrieves.
	if n := s.conn.notify.notifier; n != nil {
		n.abort()
	}
	s.fileSystem.OnUnmount()

	s.stateMu.Lock()
	s.state = stateClosed
	s.stateMu.Unlock()
	close(s.done)
}

// writeNotify writes a notification.
func (s *Server) writeNotify(iov [][]byte) (int, syscall.Errno) {
	if s.notifyStopped.Load() {
		return 0, syscall.EINTR
	}
	return s.fuseFD.writev(iov)
}

func (s *Server) handleInit() Status {
	// The first request should be INIT; read it synchronously.
	req, _, errNo := s.fuseFD.readRequest()

	if errNo != OK || req == nil {
		return errNo
	}
	if code := s.handleRequest(req); !code.Ok() {
		return code
	}
	s.applyNegotiated()
	return OK
}

// loop is the FUSE event loop. The simplistic way of calling this is
// with singleReader=true, which has a single goroutine reading the
// device, and spawning a new goroutine for each request. It is
// however 2x slower than processing the request inline with the
// reader. The latter requires more logic, because whenever we start
// processing the request, we have to make sure a new routine is
// spawned to read the device.
//
// Benchmark results i5-8350 pinned at 2Ghz:
//
// singleReader = true
//
// BenchmarkGoFuseRead            	     954	   1137408 ns/op	1843.80 MB/s	    5459 B/op	     173 allocs/op
// BenchmarkGoFuseRead-2          	    1327	    798635 ns/op	2625.92 MB/s	    5072 B/op	     169 allocs/op
// BenchmarkGoFuseStat            	    1530	    750944 ns/op
// BenchmarkGoFuseStat-2          	    8455	    120091 ns/op
// BenchmarkGoFuseReaddir         	     741	   1561004 ns/op
// BenchmarkGoFuseReaddir-2       	    2508	    599821 ns/op
//
// singleReader = false
//
// BenchmarkGoFuseRead            	    1890	    671576 ns/op	3122.73 MB/s	    5393 B/op	     136 allocs/op
// BenchmarkGoFuseRead-2          	    2948	    429578 ns/op	4881.88 MB/s	   32235 B/op	     157 allocs/op
// BenchmarkGoFuseStat            	    7886	    153546 ns/op
// BenchmarkGoFuseStat-2          	    9310	    121332 ns/op
// BenchmarkGoFuseReaddir         	    4074	    361568 ns/op
// BenchmarkGoFuseReaddir-2       	    3511	    319765 ns/op
func (s *Server) loop() {
	defer s.loopDone()
exit:
	for {
		req, needReader, errNo := s.fuseFD.readRequest()
		switch errNo {
		case OK:
			if req == nil {
				break exit
			}
		case ENOENT:
			continue
		case ENODEV:
			// Mount was killed. The obvious place to
			// cancel outstanding requests is when the
			// server closes, but that reader might be blocked.
			s.cancelAll()
			if s.opts.Debug {
				s.opts.Logger.Printf("received ENODEV (unmount request), thread exiting")
			}
			s.markDead()
			break exit
		default: // some other error?
			s.opts.Logger.Printf("Failed to read from fuse conn: %v", errNo)
			s.markDead()
			break exit
		}

		if !s.singleReader && needReader {
			s.addLoop()
			go s.loop()
		}
		if s.singleReader && s.fuseFD.canAcceptAnother() {
			s.addLoop()
			go func() {
				defer s.loopDone()
				s.handleRequest(req)
			}()
			continue
		}
		s.handleRequest(req)
	}
}

func (s *Server) handleRequest(req *requestAlloc) Status {
	defer s.fuseFD.returnRequest(req)
	if s.opts.SingleThreaded {
		s.requestProcessingMu.Lock()
		defer s.requestProcessingMu.Unlock()
	}

	h, inSize, outSize, outPayloadSize, code := parseRequest(req.inputBuf, s.conn.out.Flags64())
	req.request.status = code
	if !code.Ok() && code != ENOSYS {
		s.opts.Logger.Printf("parseRequest: %v", code)
	}
	if code.Ok() {
		req.request.status = req.splitPayload(inSize, int(h.InputSize))
		if !req.request.status.Ok() {
			s.opts.Logger.Printf("op %s: bad request extension: %v", h.Name, req.request.status)
		}
	}
	req.outHeaderBuf = req.outHeaderInline[:]
	req.outDataBuf = req.outDataInline[:outSize]
	clear(req.outHeaderBuf)
	clear(req.outDataBuf)
	if outPayloadSize > 0 {
		req.outPayload = s.buffers.AllocBuffer(uint32(outPayloadSize))
		req.bufferPoolOutputBuf = req.outPayload
	}
	s.protocolServer.handleRequest(h, &req.request)
	if req.suppressReply {
		return OK
	}
	errno := s.fuseFD.write(req)
	if errno != 0 {
		// Ignore ENOENT for INTERRUPT responses which
		// indicates that the referred request is no longer
		// known by the kernel. This is a normal if the
		// referred request already has completed.
		//
		// Ignore ENOENT for RELEASE responses.  When the FS
		// is unmounted directly after a file close, the
		// device can go away while we are still processing
		// RELEASE. This is because RELEASE is analogous to
		// FORGET, and is not synchronized with the calling
		// process, but does require a response.
		if s.opts.Debug || !(errno == ENOENT && (req.inHeader().Opcode == _OP_INTERRUPT ||
			req.inHeader().Opcode == _OP_RELEASEDIR ||
			req.inHeader().Opcode == _OP_RELEASE)) {
			s.opts.Logger.Printf("writer: Write/Writev failed, err: %v. opcode: %v",
				errno, operationName(req.inHeader().Opcode))
		}
	}
	return errno
}

// WaitMount waits for the first request to be served. Use this to
// avoid racing between accessing the (empty or not yet mounted)
// mountpoint, and the OS trying to setup the user-space mount.
func (s *Server) WaitMount() error {
	err := <-s.ready
	if err != nil {
		return err
	}
	if s.mount == nil || s.mount.magic() {
		// We don't know the real mountpoint, so we cannot run
		// the poll hack.
		return nil
	}
	return pollHack(s.mount.path)
}

// parseFuseFd checks if `mountPoint` is the special form /dev/fd/N (with N >= 0),
// and returns N in this case. Returns -1 otherwise.
func parseFuseFd(mountPoint string) (fd int) {
	dir, file := path.Split(mountPoint)
	if dir != "/dev/fd/" {
		return -1
	}
	fd, err := strconv.Atoi(file)
	if err != nil || fd <= 0 {
		return -1
	}
	return fd
}

// errRecoverSplice is returned by trySplice when the caller should
// fall back to to pread/read without logging.
var errRecoverSplice = errors.New("splice failed; must fallback")
