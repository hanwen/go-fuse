// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"path"
	"reflect"
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

	// defaultMaxWrite is the default value for MountOptions.MaxWrite
	defaultMaxWrite = 128 * 1024 // 128 kiB

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
func (ms *Server) KernelSettings() *InitIn {
	s := ms.conn.in

	return &s
}

const _MAX_NAME_LEN = 20

// Unmount unmounts the file system (see Mount.Unmount), and
// waits for the server to exit. After the Server is unmounted, it
// should be discarded. This function is idempotent.
func (ms *Server) Unmount() error {
	if ms.mount == nil {
		return errors.New("fuse: server has no mount")
	}
	if err := ms.mount.Unmount(); err != nil {
		return err
	}
	// The kernel has aborted the connection.
	ms.cancelAll()
	ms.markDead()
	<-ms.done
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

func (o *MountOptions) setDefaults(fs RawFileSystem) {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	if o.MaxWrite < 0 {
		o.MaxWrite = 0
	}
	if o.MaxWrite == 0 {
		o.MaxWrite = defaultMaxWrite
	}
	kernelMaxWrite := getMaxWrite()
	if o.MaxWrite > kernelMaxWrite {
		o.MaxWrite = kernelMaxWrite
	}
	if o.MaxInflightRequestBytes <= 0 {
		o.MaxInflightRequestBytes = math.MaxInt
	}
	if o.MaxStackDepth == 0 {
		o.MaxStackDepth = 1
	}
	if o.Name == "" && fs != nil {
		var name string
		if s, ok := fs.(fmt.Stringer); ok {
			name = s.String()
		} else {
			t := reflect.TypeOf(fs)
			for t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			name = t.Name()
		}
		l := min(len(name), _MAX_NAME_LEN)
		o.Name = strings.Replace(name[:l], ",", ";", -1)
	}
	if o.PanicHandler == nil {
		l := o.Logger
		o.PanicHandler = func(obj any) Status {
			return defaultPanicHandler(l, obj)
		}
	}

	for _, s := range []struct {
		flag bool
		mask uint64
	}{
		{o.SyncRead, CAP_ASYNC_READ},
		{o.DisableReadDirPlus, CAP_READDIRPLUS},
		{!o.IDMappedMount, CAP_ALLOW_IDMAP},
	} {
		if s.flag {
			o.DisabledCapabilities |= s.mask
		}
	}
}

func defaultPanicHandler(logger *log.Logger, obj any) Status {
	const size = 64 << 10
	buf := make([]byte, size)
	buf = buf[:runtime.Stack(buf, false)]
	logger.Printf("panic in FS handler: %v\n%s", obj, buf)
	return EIO
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

// NewServerFromDev creates a server for the FUSE device d, and negotiates
// the connection with the kernel. It takes ownership of d, also if it
// fails. m is where d is mounted, for Unmount and WaitMount. It may
// be nil; Unmount then fails. The fields of opts that only apply to
// mounting are ignored.
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
	var err error
	ms.fuseFD, err = ms.newFuseFD(d.release(), maxReaders)
	if err != nil {
		return nil, err
	}
	ms.setMaxWrite(o.MaxWrite)
	ms.conn.notify = newNotify(ms.writeNotify, o)
	ms.conn.backing = ms.fdBacking()

	if code := ms.handleInit(); !code.Ok() {
		ms.fuseFD.close()
		// TODO - unmount as well?
		return nil, fmt.Errorf("init: %s", code)
	}
	return ms, nil
}

// setMaxWrite sizes the read buffers for requests carrying up to
// maxWrite bytes of data. It must be called while no request is read.
func (ms *Server) setMaxWrite(maxWrite int) {
	readBufSize, readBufBytes, _ := requestAccountingSizes(maxWrite)
	ms.readPool = sync.Pool{
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
	ms.fuseFD.readBufBytes = readBufBytes
}

// applyNegotiated sizes the read buffers and enables splice for
// the negotiated connection.
func (ms *Server) applyNegotiated() {
	ms.setMaxWrite(int(ms.conn.out.MaxWrite))
	if ms.conn.out.Minor >= 13 {
		ms.setSplice()
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
func (ms *Server) DebugData() string {
	var r int
	ms.fuseFD.reqMu.Lock()
	r = ms.fuseFD.reqReaders
	ms.fuseFD.reqMu.Unlock()

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
func (ms *Server) Serve() {
	if !ms.serving.CompareAndSwap(false, true) {
		// Calling Serve() multiple times leads to a panic on unmount and fun
		// debugging sessions ( https://github.com/hanwen/go-fuse/issues/512 ).
		// Catch it early.
		log.Panic("Serve() must only be called once, you have called it a second time")
	}
	if err := ms.Start(); err != nil {
		ms.opts.Logger.Printf("Serve: %v", err)
	}
	ms.Wait()
}

// Wait waits until the server is closed.
func (ms *Server) Wait() {
	<-ms.done
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
func (ms *Server) Start() error {
	ms.lifeMu.Lock()
	defer ms.lifeMu.Unlock()
	ms.stateMu.Lock()
	defer ms.stateMu.Unlock()
	switch ms.state {
	case stateRunning:
		return nil
	case stateClosing, stateClosed:
		return errServerClosed
	}
	if ms.dead {
		return errServerClosed
	}
	ms.fuseFD.startReading()
	ms.state = stateRunning
	ms.notifyStopped.Store(false)
	ms.loops++
	go ms.loop()
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
func (ms *Server) Stop(ctx context.Context) error {
	ms.lifeMu.Lock()
	defer ms.lifeMu.Unlock()
	return ms.stop(ctx)
}

func (ms *Server) stop(ctx context.Context) error {
	ms.stateMu.Lock()
	switch ms.state {
	case stateClosing, stateClosed:
		ms.stateMu.Unlock()
		return errServerClosed
	}
	ms.state = stateStopped
	ms.notifyStopped.Store(true)
	ms.stateMu.Unlock()

	var err error
	if n := ms.conn.notify.notifier; n != nil {
		err = n.waitRetrieves(ctx)
	}
	ms.fuseFD.stopReading()
	if err == nil {
		err = ms.waitLoopsZero(ctx)
	}
	return err
}

// Shutdown stops the server as Stop does, and then closes it: it
// closes this server's file descriptor, and calls
// RawFileSystem.OnUnmount. It does not unmount the file system; if
// no other process has the FUSE device open, the kernel aborts the
// connection. If Stop fails, the server stays stopped.
func (ms *Server) Shutdown(ctx context.Context) error {
	ms.lifeMu.Lock()
	defer ms.lifeMu.Unlock()
	if err := ms.stop(ctx); err != nil {
		if err == errServerClosed {
			<-ms.done
			return nil
		}
		return err
	}
	ms.finish()
	return nil
}

// addLoop counts a goroutine that reads or serves requests. The
// caller must be counted itself, so the count is not zero.
func (ms *Server) addLoop() {
	ms.stateMu.Lock()
	ms.loops++
	ms.stateMu.Unlock()
}

// loopDone is called when a goroutine counted by addLoop exits.
func (ms *Server) loopDone() {
	ms.stateMu.Lock()
	ms.loops--
	finish := ms.checkLoopsZeroLocked()
	ms.stateMu.Unlock()
	if finish {
		ms.finish()
	}
}

// markDead marks reading as failed for good, so readers exit, and
// the server closes once they have.
func (ms *Server) markDead() {
	ms.stateMu.Lock()
	ms.dead = true
	ms.fuseFD.stopReading()
	finish := ms.checkLoopsZeroLocked()
	ms.stateMu.Unlock()
	if finish {
		ms.finish()
	}
}

// checkLoopsZeroLocked closes loopsZero if no goroutine reads or serves
// requests. It returns whether the server should then be closed,
// because reading has failed.
func (ms *Server) checkLoopsZeroLocked() bool {
	if ms.loops > 0 {
		return false
	}
	if ms.loopsZero != nil {
		close(ms.loopsZero)
		ms.loopsZero = nil
	}
	return ms.dead
}

// waitLoopsZero waits until no goroutine reads or serves requests.
func (ms *Server) waitLoopsZero(ctx context.Context) error {
	ms.stateMu.Lock()
	if ms.loops == 0 {
		ms.stateMu.Unlock()
		return nil
	}
	if ms.loopsZero == nil {
		ms.loopsZero = make(chan struct{})
	}
	ch := ms.loopsZero
	ms.stateMu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// finish closes the server once. It must be called with no request
// being read or served.
func (ms *Server) finish() {
	ms.stateMu.Lock()
	if ms.state == stateClosing || ms.state == stateClosed {
		ms.stateMu.Unlock()
		<-ms.done
		return
	}
	ms.state = stateClosing
	ms.stateMu.Unlock()

	ms.fuseFD.close()
	// shutdown in-flight cache retrieves.
	if n := ms.conn.notify.notifier; n != nil {
		n.abort()
	}
	ms.fileSystem.OnUnmount()

	ms.stateMu.Lock()
	ms.state = stateClosed
	ms.stateMu.Unlock()
	close(ms.done)
}

// writeNotify writes a notification.
func (ms *Server) writeNotify(iov [][]byte) (int, syscall.Errno) {
	if ms.notifyStopped.Load() {
		return 0, syscall.EINTR
	}
	return ms.fuseFD.writev(iov)
}

func (ms *Server) handleInit() Status {
	// The first request should be INIT; read it synchronously.
	req, _, errNo := ms.fuseFD.readRequest()

	if errNo != OK || req == nil {
		return errNo
	}
	if code := ms.handleRequest(req); !code.Ok() {
		return code
	}
	ms.applyNegotiated()
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
func (ms *Server) loop() {
	defer ms.loopDone()
exit:
	for {
		req, needReader, errNo := ms.fuseFD.readRequest()
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
			ms.cancelAll()
			if ms.opts.Debug {
				ms.opts.Logger.Printf("received ENODEV (unmount request), thread exiting")
			}
			ms.markDead()
			break exit
		default: // some other error?
			ms.opts.Logger.Printf("Failed to read from fuse conn: %v", errNo)
			ms.markDead()
			break exit
		}

		if !ms.singleReader && needReader {
			ms.addLoop()
			go ms.loop()
		}
		if ms.singleReader && ms.fuseFD.canAcceptAnother() {
			ms.addLoop()
			go func() {
				defer ms.loopDone()
				ms.handleRequest(req)
			}()
			continue
		}
		ms.handleRequest(req)
	}
}

func (ms *Server) handleRequest(req *requestAlloc) Status {
	defer ms.fuseFD.returnRequest(req)
	if ms.opts.SingleThreaded {
		ms.requestProcessingMu.Lock()
		defer ms.requestProcessingMu.Unlock()
	}

	h, inSize, outSize, outPayloadSize, code := parseRequest(req.inputBuf, ms.conn.out.Flags64())
	req.request.status = code
	if !code.Ok() && code != ENOSYS {
		ms.opts.Logger.Printf("parseRequest: %v", code)
	}
	if code.Ok() {
		req.request.status = req.splitPayload(inSize, int(h.InputSize))
		if !req.request.status.Ok() {
			ms.opts.Logger.Printf("op %s: bad request extension: %v", h.Name, req.request.status)
		}
	}
	req.outHeaderBuf = req.outHeaderInline[:]
	req.outDataBuf = req.outDataInline[:outSize]
	clear(req.outHeaderBuf)
	clear(req.outDataBuf)
	if outPayloadSize > 0 {
		req.outPayload = ms.buffers.AllocBuffer(uint32(outPayloadSize))
		req.bufferPoolOutputBuf = req.outPayload
	}
	ms.protocolServer.handleRequest(h, &req.request)
	if req.suppressReply {
		return OK
	}
	errno := ms.fuseFD.write(req)
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
		if ms.opts.Debug || !(errno == ENOENT && (req.inHeader().Opcode == _OP_INTERRUPT ||
			req.inHeader().Opcode == _OP_RELEASEDIR ||
			req.inHeader().Opcode == _OP_RELEASE)) {
			ms.opts.Logger.Printf("writer: Write/Writev failed, err: %v. opcode: %v",
				errno, operationName(req.inHeader().Opcode))
		}
	}
	return errno
}

// SupportsVersion returns true if the kernel supports the given
// protocol version or newer.
func (in *InitIn) SupportsVersion(maj, min uint32) bool {
	return in.Major > maj || (in.Major == maj && in.Minor >= min)
}

// SupportsNotify returns whether a certain notification type is
// supported. Pass any of the NOTIFY_* types as argument.
func (in *InitIn) SupportsNotify(notifyType int) bool {
	switch notifyType {
	case NOTIFY_INVAL_ENTRY:
		return in.SupportsVersion(7, 12)
	case NOTIFY_INVAL_INODE:
		return in.SupportsVersion(7, 12)
	case NOTIFY_STORE_CACHE, NOTIFY_RETRIEVE_CACHE:
		return in.SupportsVersion(7, 15)
	case NOTIFY_DELETE:
		return in.SupportsVersion(7, 18)
	case NOTIFY_PRUNE:
		return in.SupportsVersion(7, 45)
	}
	return false
}

// WaitMount waits for the first request to be served. Use this to
// avoid racing between accessing the (empty or not yet mounted)
// mountpoint, and the OS trying to setup the user-space mount.
func (ms *Server) WaitMount() error {
	err := <-ms.ready
	if err != nil {
		return err
	}
	if ms.mount == nil || ms.mount.magic() {
		// We don't know the real mountpoint, so we cannot run
		// the poll hack.
		return nil
	}
	return pollHack(ms.mount.path)
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
