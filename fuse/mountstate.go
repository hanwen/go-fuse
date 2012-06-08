package fuse

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
	"github.com/hanwen/go-fuse/splice"
)

const (
	// The kernel caps writes at 128k.
	MAX_KERNEL_WRITE = 128 * 1024
)

// MountState contains the logic for reading from the FUSE device and
// translating it to RawFileSystem interface calls.
type MountState struct {
	// Empty if unmounted.
	mountPoint string
	fileSystem RawFileSystem

	// I/O with kernel and daemon.
	mountFile *os.File

	// Dump debug info onto stdout.
	Debug bool

	latencies *LatencyMap

	opts           *MountOptions
	kernelSettings raw.InitIn

	reqMu               sync.Mutex
	reqPool             []*request
	readPool            [][]byte
	reqReaders          int
	outstandingReadBufs int

	canSplice bool
	loops     sync.WaitGroup
}

func (ms *MountState) KernelSettings() raw.InitIn {
	return ms.kernelSettings
}

func (ms *MountState) MountPoint() string {
	return ms.mountPoint
}

const _MAX_NAME_LEN = 20

// Mount filesystem on mountPoint.
func (ms *MountState) Mount(mountPoint string, opts *MountOptions) error {
	if opts == nil {
		opts = &MountOptions{
			MaxBackground: _DEFAULT_BACKGROUND_TASKS,
		}
	}
	o := *opts
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
	ms.opts = &o

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

	file, mp, err := mount(mountPoint, strings.Join(optStrs, ","))
	if err != nil {
		return err
	}
	initParams := RawFsInit{
		InodeNotify: func(n *raw.NotifyInvalInodeOut) Status {
			return ms.writeInodeNotify(n)
		},
		EntryNotify: func(parent uint64, n string) Status {
			return ms.writeEntryNotify(parent, n)
		},
	}
	ms.fileSystem.Init(&initParams)
	ms.mountPoint = mp
	ms.mountFile = file
	return nil
}

func (ms *MountState) SetRecordStatistics(record bool) {
	if record {
		ms.latencies = NewLatencyMap()
	} else {
		ms.latencies = nil
	}
}

func (ms *MountState) Unmount() (err error) {
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
	// Wait for event loops to exit.
	ms.loops.Wait()
	ms.mountPoint = ""
	return err
}

func NewMountState(fs RawFileSystem) *MountState {
	ms := new(MountState)
	ms.mountPoint = ""
	ms.fileSystem = fs
	return ms
}

func (ms *MountState) Latencies() *LatencyMap {
	return ms.latencies
}

func (ms *MountState) BufferPoolStats() string {
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
func (ms *MountState) readRequest(exitIdle bool) (req *request, code Status) {
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

	n, err := ms.mountFile.Read(dest)
	if err != nil {
		code = ToStatus(err)
		ms.reqMu.Lock()
		ms.reqPool = append(ms.reqPool, req)
		ms.reqReaders--
		ms.reqMu.Unlock()
		return nil, code
	}

	if ms.latencies != nil {
		req.startNs = time.Now().UnixNano()
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

func (ms *MountState) returnRequest(req *request) {
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

func (ms *MountState) recordStats(req *request) {
	if ms.latencies != nil {
		dt := time.Now().UnixNano() - req.startNs
		opname := operationName(req.inHeader.Opcode)
		ms.latencies.Add(opname, dt)
	}
}

// Loop initiates the FUSE loop. Normally, callers should run Loop()
// and wait for it to exit, but tests will want to run this in a
// goroutine.
//
// Each filesystem operation executes in a separate goroutine.
func (ms *MountState) Loop() {
	ms.loops.Add(1)
	ms.loop(false)
	ms.loops.Wait()
	ms.mountFile.Close()
}

func (ms *MountState) loop(exitIdle bool) {
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

func (ms *MountState) handleRequest(req *request) {
	req.parse()
	if req.handler == nil {
		req.status = ENOSYS
	}

	if req.status.Ok() && ms.Debug {
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

func (ms *MountState) AllocOut(req *request, size uint32) []byte {
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

func (ms *MountState) write(req *request) Status {
	// Forget does not wait for reply.
	if req.inHeader.Opcode == _OP_FORGET || req.inHeader.Opcode == _OP_BATCH_FORGET {
		return OK
	}

	header := req.serializeHeader(req.flatDataSize())
	if ms.Debug {
		log.Println(req.OutputDebug())
	}

	if header == nil {
		return OK
	}

	if req.flatDataSize() == 0 {
		_, err := ms.mountFile.Write(header)
		return ToStatus(err)
	}

	if req.fdData != nil {
		if err := ms.TrySplice(header, req, req.fdData); err == nil {
			return OK
		} else {
			log.Println("TrySplice:", err)
			sz := req.flatDataSize()
			buf := ms.AllocOut(req, uint32(sz))
			req.flatData = req.fdData.Bytes(buf)
			header = req.serializeHeader(len(req.flatData))
		}
	}

	_, err := Writev(int(ms.mountFile.Fd()), [][]byte{header, req.flatData})
	return ToStatus(err)
}

func (ms *MountState) TrySplice(header []byte, req *request, fdData *ReadResultFd) error {
	pair, err := splice.Get()
	if err != nil {
		return err
	}
	defer splice.Done(pair)

	total := len(header) + fdData.Size()
	if !pair.Grow(total) {
		return fmt.Errorf("splice.Grow failed.")
	}

	_, err = pair.Write(header)
	if err != nil {
		return err
	}

	var n int
	if fdData.Off < 0 {
		n, err = pair.LoadFrom(fdData.Fd, fdData.Size())
	} else {
		n, err = pair.LoadFromAt(fdData.Fd, fdData.Size(), fdData.Off)
	}
	if err == io.EOF || (err == nil && n < fdData.Size()) {
		discard := make([]byte, len(header))
		_, err = pair.Read(discard)
		if err != nil {
			return err
		}

		header = req.serializeHeader(n)

		newFd := ReadResultFd{
			Fd:  pair.ReadFd(),
			Off: -1,
			Sz:  n,
		}
		return ms.TrySplice(header, req, &newFd)
	}

	if err != nil {
		// TODO - extract the data from splice.
		return err
	}

	if n != fdData.Size() {
		return fmt.Errorf("wrote %d, want %d", n, fdData.Size())
	}

	_, err = pair.WriteTo(ms.mountFile.Fd(), total)
	if err != nil {
		return fmt.Errorf("splice write: %v", err)
	}
	return nil
}

func (ms *MountState) writeInodeNotify(entry *raw.NotifyInvalInodeOut) Status {
	req := request{
		inHeader: &raw.InHeader{
			Opcode: _OP_NOTIFY_INODE,
		},
		handler: operationHandlers[_OP_NOTIFY_INODE],
		status:  raw.NOTIFY_INVAL_INODE,
	}
	req.outData = unsafe.Pointer(entry)
	result := ms.write(&req)

	if ms.Debug {
		log.Println("Response: INODE_NOTIFY", result)
	}
	return result
}

func (ms *MountState) writeEntryNotify(parent uint64, name string) Status {
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
	result := ms.write(&req)

	if ms.Debug {
		log.Printf("Response: ENTRY_NOTIFY: %v", result)
	}
	return result
}

var defaultBufferPool BufferPool

func init() {
	defaultBufferPool = NewBufferPool()
}
