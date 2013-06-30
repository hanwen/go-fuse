package fuse

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

const (
	// The kernel caps writes at 128k.
	MAX_KERNEL_WRITE = 128 * 1024
)

// Server contains the logic for reading from the FUSE device and
// translating it to RawFileSystem interface calls.
type Server struct {
	// Empty if unmounted.
	mountPoint string
	fileSystem RawFileSystem

	// I/O with kernel and daemon.
	mountFd int

	// Dump debug info onto stdout.
	debug bool

	latencies LatencyMap

	opts *MountOptions

	started chan struct{}

	reqMu               sync.Mutex
	reqPool             []*request
	readPool            [][]byte
	reqReaders          int
	outstandingReadBufs int
	kernelSettings      raw.InitIn

	canSplice bool
	loops     sync.WaitGroup
}

func (ms *Server) SetDebug(dbg bool) {
	ms.debug = dbg
}

// KernelSettings returns the Init message from the kernel, so
// filesystems can adapt to availability of features of the kernel
// driver.
func (ms *Server) KernelSettings() raw.InitIn {
	ms.reqMu.Lock()
	s := ms.kernelSettings
	ms.reqMu.Unlock()

	return s
}

const _MAX_NAME_LEN = 20

// This type may be provided for recording latencies of each FUSE
// operation.
type LatencyMap interface {
	Add(name string, dt time.Duration)
}

// RecordLatencies switches on collection of timing for each request
// coming from the kernel.P assing a nil argument switches off the
func (ms *Server) RecordLatencies(l LatencyMap) {
	ms.latencies = l
}

// Unmount calls fusermount -u on the mount. This has the effect of
// shutting down the filesystem. After the Server is unmounted, it
// should be discarded.
func (ms *Server) Unmount() (err error) {
	if ms.mountPoint == "" {
		return nil
	}
	delay := time.Duration(0)
	for try := 0; try < 5; try++ {
		err = unmount(ms.mountPoint)
		if err == nil {
			break
		}

		// Sleep for a bit. This is not pretty, but there is
		// no way we can be certain that the kernel thinks all
		// open files have already been closed.
		delay = 2*delay + 5*time.Millisecond
		time.Sleep(delay)
	}
	if err != nil {
		return
	}
	// Wait for event loops to exit.
	ms.loops.Wait()
	ms.mountPoint = ""
	return err
}

// NewServer creates a server and attaches it to the given directory.
func NewServer(fs RawFileSystem, mountPoint string, opts *MountOptions) (*Server, error) {
	if opts == nil {
		opts = &MountOptions{
			MaxBackground: _DEFAULT_BACKGROUND_TASKS,
		}
	}
	o := *opts
	if o.SingleThreaded {
		fs = NewLockingRawFileSystem(fs)
	}

	if o.Buffers == nil {
		o.Buffers = defaultBufferPool
	}
	if o.MaxWrite < 0 {
		o.MaxWrite = 0
	}
	if o.MaxWrite == 0 {
		o.MaxWrite = 1 << 16
	}
	if o.MaxWrite > MAX_KERNEL_WRITE {
		o.MaxWrite = MAX_KERNEL_WRITE
	}
	opts = &o
	ms := &Server{
		fileSystem: fs,
		started:    make(chan struct{}),
		opts:       &o,
	}

	optStrs := opts.Options
	if opts.AllowOther {
		optStrs = append(optStrs, "allow_other")
	}

	name := opts.Name
	if name == "" {
		name = ms.fileSystem.String()
		l := len(name)
		if l > _MAX_NAME_LEN {
			l = _MAX_NAME_LEN
		}
		name = strings.Replace(name[:l], ",", ";", -1)
	}
	optStrs = append(optStrs, "subtype="+name)

	mountPoint = filepath.Clean(mountPoint)
	if !filepath.IsAbs(mountPoint) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		mountPoint = filepath.Clean(filepath.Join(cwd, mountPoint))
	}
	fd, err := mount(mountPoint, strings.Join(optStrs, ","))
	if err != nil {
		return nil, err
	}
	initParams := RawFsInit{
		InodeNotify: func(n *raw.NotifyInvalInodeOut) Status {
			return ms.writeInodeNotify(n)
		},
		EntryNotify: func(parent uint64, n string) Status {
			return ms.writeEntryNotify(parent, n)
		},
		DeleteNotify: func(parent uint64, child uint64, n string) Status {
			return ms.writeDeleteNotify(parent, child, n)
		},
	}
	ms.fileSystem.Init(&initParams)
	ms.mountPoint = mountPoint
	ms.mountFd = fd
	return ms, nil
}

// DebugData returns internal status information for debugging
// purposes.
func (ms *Server) DebugData() string {
	s := ms.opts.Buffers.String()

	var r int
	ms.reqMu.Lock()
	r = len(ms.readPool) + ms.reqReaders
	ms.reqMu.Unlock()

	s += fmt.Sprintf(" read buffers: %d (sz %d )",
		r, ms.opts.MaxWrite/PAGESIZE+1)
	return s
}

// What is a good number?  Maybe the number of CPUs?
const _MAX_READERS = 2

// Returns a new request, or error. In case exitIdle is given, returns
// nil, OK if we have too many readers already.
func (ms *Server) readRequest(exitIdle bool) (req *request, code Status) {
	var dest []byte

	ms.reqMu.Lock()
	if ms.reqReaders > _MAX_READERS {
		ms.reqMu.Unlock()
		return nil, OK
	}
	l := len(ms.reqPool)
	if l > 0 {
		req = ms.reqPool[l-1]
		ms.reqPool = ms.reqPool[:l-1]
	} else {
		req = new(request)
	}
	l = len(ms.readPool)
	if l > 0 {
		dest = ms.readPool[l-1]
		ms.readPool = ms.readPool[:l-1]
	} else {
		dest = make([]byte, ms.opts.MaxWrite+PAGESIZE)
	}
	ms.outstandingReadBufs++
	ms.reqReaders++
	ms.reqMu.Unlock()

	n, err := syscall.Read(ms.mountFd, dest)
	if err != nil {
		code = ToStatus(err)
		ms.reqMu.Lock()
		ms.reqPool = append(ms.reqPool, req)
		ms.reqReaders--
		ms.reqMu.Unlock()
		return nil, code
	}

	if ms.latencies != nil {
		req.startTime = time.Now()
	}
	gobbled := req.setInput(dest[:n])

	ms.reqMu.Lock()
	if !gobbled {
		ms.outstandingReadBufs--
		ms.readPool = append(ms.readPool, dest)
		dest = nil
	}
	ms.reqReaders--
	if ms.reqReaders <= 0 {
		ms.loops.Add(1)
		go ms.loop(true)
	}
	ms.reqMu.Unlock()

	return req, OK
}

// returnRequest returns a request to the pool of unused requests.
func (ms *Server) returnRequest(req *request) {
	ms.recordStats(req)

	if req.bufferPoolOutputBuf != nil {
		ms.opts.Buffers.FreeBuffer(req.bufferPoolOutputBuf)
		req.bufferPoolOutputBuf = nil
	}

	req.clear()
	ms.reqMu.Lock()
	if req.bufferPoolOutputBuf != nil {
		ms.readPool = append(ms.readPool, req.bufferPoolInputBuf)
		ms.outstandingReadBufs--
		req.bufferPoolInputBuf = nil
	}
	ms.reqPool = append(ms.reqPool, req)
	ms.reqMu.Unlock()
}

func (ms *Server) recordStats(req *request) {
	if ms.latencies != nil {
		dt := time.Now().Sub(req.startTime)
		opname := operationName(req.inHeader.Opcode)
		ms.latencies.Add(opname, dt)
	}
}

// Serve initiates the FUSE loop. Normally, callers should run Serve()
// and wait for it to exit, but tests will want to run this in a
// goroutine.
//
// Each filesystem operation executes in a separate goroutine.
func (ms *Server) Serve() {
	ms.loops.Add(1)
	ms.loop(false)
	ms.loops.Wait()

	ms.reqMu.Lock()
	syscall.Close(ms.mountFd)
	ms.reqMu.Unlock()
}

func (ms *Server) loop(exitIdle bool) {
	defer ms.loops.Done()
exit:
	for {
		req, errNo := ms.readRequest(exitIdle)
		switch errNo {
		case OK:
			if req == nil {
				break exit
			}
		case ENOENT:
			continue
		case ENODEV:
			// unmount
			break exit
		default: // some other error?
			log.Printf("Failed to read from fuse conn: %v", errNo)
			break exit
		}

		ms.handleRequest(req)
	}
}

func (ms *Server) handleRequest(req *request) {
	req.parse()
	if req.handler == nil {
		req.status = ENOSYS
	}

	if req.status.Ok() && ms.debug {
		log.Println(req.InputDebug())
	}

	if req.status.Ok() && req.handler.Func == nil {
		log.Printf("Unimplemented opcode %v", operationName(req.inHeader.Opcode))
		req.status = ENOSYS
	}

	if req.status.Ok() {
		req.handler.Func(ms, req)
	}

	errNo := ms.write(req)
	if errNo != 0 {
		log.Printf("writer: Write/Writev failed, err: %v. opcode: %v",
			errNo, operationName(req.inHeader.Opcode))
	}
	ms.returnRequest(req)
}

func (ms *Server) allocOut(req *request, size uint32) []byte {
	if cap(req.bufferPoolOutputBuf) >= int(size) {
		req.bufferPoolOutputBuf = req.bufferPoolOutputBuf[:size]
		return req.bufferPoolOutputBuf
	}
	if req.bufferPoolOutputBuf != nil {
		ms.opts.Buffers.FreeBuffer(req.bufferPoolOutputBuf)
	}
	req.bufferPoolOutputBuf = ms.opts.Buffers.AllocBuffer(size)
	return req.bufferPoolOutputBuf
}

func (ms *Server) write(req *request) Status {
	// Forget does not wait for reply.
	if req.inHeader.Opcode == _OP_FORGET || req.inHeader.Opcode == _OP_BATCH_FORGET {
		return OK
	}

	header := req.serializeHeader(req.flatDataSize())
	if ms.debug {
		log.Println(req.OutputDebug())
	}

	if header == nil {
		return OK
	}

	s := ms.systemWrite(req, header)
	if req.inHeader.Opcode == _OP_INIT {
		close(ms.started)
	}
	return s
}

func (ms *Server) writeInodeNotify(entry *raw.NotifyInvalInodeOut) Status {
	req := request{
		inHeader: &raw.InHeader{
			Opcode: _OP_NOTIFY_INODE,
		},
		handler: operationHandlers[_OP_NOTIFY_INODE],
		status:  raw.NOTIFY_INVAL_INODE,
	}
	req.outData = unsafe.Pointer(entry)

	// Protect against concurrent close.
	ms.reqMu.Lock()
	result := ms.write(&req)
	ms.reqMu.Unlock()

	if ms.debug {
		log.Println("Response: INODE_NOTIFY", result)
	}
	return result
}

func (ms *Server) writeDeleteNotify(parent uint64, child uint64, name string) Status {
	if ms.kernelSettings.Minor < 18 {
		return ms.writeEntryNotify(parent, name)
	}

	req := request{
		inHeader: &raw.InHeader{
			Opcode: _OP_NOTIFY_DELETE,
		},
		handler: operationHandlers[_OP_NOTIFY_DELETE],
		status:  raw.NOTIFY_INVAL_DELETE,
	}
	entry := &raw.NotifyInvalDeleteOut{
		Parent:  parent,
		Child:   child,
		NameLen: uint32(len(name)),
	}

	// Many versions of FUSE generate stacktraces if the
	// terminating null byte is missing.
	nameBytes := make([]byte, len(name)+1)
	copy(nameBytes, name)
	nameBytes[len(nameBytes)-1] = '\000'
	req.outData = unsafe.Pointer(entry)
	req.flatData = nameBytes

	// Protect against concurrent close.
	ms.reqMu.Lock()
	result := ms.write(&req)
	ms.reqMu.Unlock()

	if ms.debug {
		log.Printf("Response: DELETE_NOTIFY: %v", result)
	}
	return result
}

func (ms *Server) writeEntryNotify(parent uint64, name string) Status {
	req := request{
		inHeader: &raw.InHeader{
			Opcode: _OP_NOTIFY_ENTRY,
		},
		handler: operationHandlers[_OP_NOTIFY_ENTRY],
		status:  raw.NOTIFY_INVAL_ENTRY,
	}
	entry := &raw.NotifyInvalEntryOut{
		Parent:  parent,
		NameLen: uint32(len(name)),
	}

	// Many versions of FUSE generate stacktraces if the
	// terminating null byte is missing.
	nameBytes := make([]byte, len(name)+1)
	copy(nameBytes, name)
	nameBytes[len(nameBytes)-1] = '\000'
	req.outData = unsafe.Pointer(entry)
	req.flatData = nameBytes

	// Protect against concurrent close.
	ms.reqMu.Lock()
	result := ms.write(&req)
	ms.reqMu.Unlock()

	if ms.debug {
		log.Printf("Response: ENTRY_NOTIFY: %v", result)
	}
	return result
}

var defaultBufferPool BufferPool

func init() {
	defaultBufferPool = NewBufferPool()
}

// WaitMount waits for the first request to be served. Use this to
// avoid racing between accessing the (empty) mountpoint, and the OS
// trying to setup the user-space mount.
func (ms *Server) WaitMount() {
	<-ms.started
}
