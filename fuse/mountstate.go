package fuse

import (
	"log"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	// bufSize should be a power of two to minimize lossage in
	// BufferPool.  The minimum is 8k, but it doesn't cost anything to
	// use a much larger buffer.
	bufSize = (1 << 16)
	maxRead = bufSize - PAGESIZE
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

	// For efficient reads and writes.
	buffers *BufferPoolImpl

	*LatencyMap

	opts           *MountOptions
	kernelSettings InitIn
}

func (me *MountState) KernelSettings() InitIn {
	return me.kernelSettings
}

func (me *MountState) MountPoint() string {
	return me.mountPoint
}

// Mount filesystem on mountPoint.
func (me *MountState) Mount(mountPoint string, opts *MountOptions) os.Error {
	if opts == nil {
		opts = &MountOptions{
			MaxBackground: _DEFAULT_BACKGROUND_TASKS,
		}
	}
	me.opts = opts

	optStrs := opts.Options
	if opts.AllowOther {
		optStrs = append(optStrs, "allow_other")
	}

	file, mp, err := mount(mountPoint, strings.Join(optStrs, ","))
	if err != nil {
		return err
	}
	initParams := RawFsInit{
		InodeNotify: func(n *NotifyInvalInodeOut) Status {
			return me.writeInodeNotify(n)
		},
		EntryNotify: func(parent uint64, n string) Status {
			return me.writeEntryNotify(parent, n)
		},
	}
	me.fileSystem.Init(&initParams)
	me.mountPoint = mp
	me.mountFile = file
	return nil
}

func (me *MountState) SetRecordStatistics(record bool) {
	if record {
		me.LatencyMap = NewLatencyMap()
	} else {
		me.LatencyMap = nil
	}
}

func (me *MountState) Unmount() os.Error {
	// Todo: flush/release all files/dirs?
	err := unmount(me.mountPoint)
	if err == nil {
		me.mountPoint = ""
		me.mountFile.Close()
		me.mountFile = nil
	}
	return err
}

func NewMountState(fs RawFileSystem) *MountState {
	me := new(MountState)
	me.mountPoint = ""
	me.fileSystem = fs
	me.buffers = NewBufferPool()
	return me
}

func (me *MountState) Latencies() map[string]float64 {
	return me.LatencyMap.Latencies(1e-3)
}

func (me *MountState) OperationCounts() map[string]int {
	return me.LatencyMap.Counts()
}

func (me *MountState) BufferPoolStats() string {
	return me.buffers.String()
}

func (me *MountState) newRequest() *request {
	return &request{
		status:   OK,
		inputBuf: me.buffers.AllocBuffer(bufSize),
	}
}

func (me *MountState) readRequest(req *request) os.Error {
	n, err := me.mountFile.Read(req.inputBuf)
	// If we start timing before the read, we may take into
	// account waiting for input into the timing.
	if me.LatencyMap != nil {
		req.startNs = time.Nanoseconds()
	}
	req.inputBuf = req.inputBuf[0:n]
	return err
}

func (me *MountState) recordStats(req *request) {
	if me.LatencyMap != nil {
		endNs := time.Nanoseconds()
		dt := endNs - req.startNs

		opname := operationName(req.inHeader.opcode)
		me.LatencyMap.AddMany(
			[]LatencyArg{
				{opname, "", dt},
				{opname + "-write", "", endNs - req.preWriteNs}})
	}
}

// Loop initiates the FUSE loop. Normally, callers should run Loop()
// and wait for it to exit, but tests will want to run this in a
// goroutine.
//
// Each filesystem operation executes in a separate goroutine.
func (me *MountState) Loop() {
	me.loop()
	me.mountFile.Close()
}

func (me *MountState) loop() {
	for {
		req := me.newRequest()
		err := me.readRequest(req)
		if err != nil {
			errNo := OsErrorToErrno(err)

			// Retry.
			if errNo == syscall.ENOENT {
				continue
			}

			if errNo == syscall.ENODEV {
				// Unmount.
				break
			}

			log.Printf("Failed to read from fuse conn: %v", err)
			break
		}

		// When closely analyzing timings, the context switch
		// generates some delay.  While unfortunate, the
		// alternative is to have a fixed goroutine pool,
		// which will lock up the FS if the daemon has too
		// many blocking calls.
		go func(r *request) {
			me.handleRequest(r)
			me.discardRequest(r)
		}(req)
	}
}

func (me *MountState) discardRequest(req *request) {
	me.buffers.FreeBuffer(req.flatData)
	me.buffers.FreeBuffer(req.inputBuf)
}

func (me *MountState) handleRequest(req *request) {
	defer me.recordStats(req)

	req.parse()
	if req.handler == nil {
		req.status = ENOSYS
	}

	if req.status.Ok() && me.Debug {
		log.Println(req.InputDebug())
	}

	if req.status.Ok() && req.handler.Func == nil {
		log.Printf("Unimplemented opcode %v", req.inHeader.opcode)
		req.status = ENOSYS
	}

	if req.status.Ok() {
		req.handler.Func(me, req)
	}

	errNo := me.write(req)
	if errNo != 0 {
		log.Printf("writer: Write/Writev %v failed, err: %v. opcode: %v",
			req.outHeaderBytes, errNo, operationName(req.inHeader.opcode))
	}
}

func (me *MountState) write(req *request) Status {
	// If we try to write OK, nil, we will get
	// error:  writer: Writev [[16 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0]]
	// failed, err: writev: no such file or directory
	if req.inHeader.opcode == _OP_FORGET {
		return OK
	}

	req.serialize()
	if me.Debug {
		log.Println(req.OutputDebug())
	}

	if me.LatencyMap != nil {
		req.preWriteNs = time.Nanoseconds()
	}

	if req.outHeaderBytes == nil {
		return OK
	}

	var err os.Error
	if req.flatData == nil {
		_, err = me.mountFile.Write(req.outHeaderBytes)
	} else {
		_, err = Writev(me.mountFile.Fd(),
			[][]byte{req.outHeaderBytes, req.flatData})
	}

	return OsErrorToErrno(err)
}

func (me *MountState) writeInodeNotify(entry *NotifyInvalInodeOut) Status {
	req := request{
		inHeader: &InHeader{
			opcode: _OP_NOTIFY_INODE,
		},
		handler: operationHandlers[_OP_NOTIFY_INODE],
		status:  NOTIFY_INVAL_INODE,
	}
	req.outData = unsafe.Pointer(entry)
	req.serialize()
	result := me.write(&req)

	if me.Debug {
		log.Println("Response: INODE_NOTIFY", result)
	}
	return result
}

func (me *MountState) writeEntryNotify(parent uint64, name string) Status {
	req := request{
		inHeader: &InHeader{
			opcode: _OP_NOTIFY_ENTRY,
		},
		handler: operationHandlers[_OP_NOTIFY_ENTRY],
		status:  NOTIFY_INVAL_ENTRY,
	}
	entry := &NotifyInvalEntryOut{
		Parent:  parent,
		NameLen: uint32(len(name)),
	}

	// Many versions of FUSE generate stacktraces if the
	// terminating null byte is missing.
	nameBytes := []byte(name + "\000")
	req.outData = unsafe.Pointer(entry)
	req.flatData = nameBytes
	req.serialize()
	result := me.write(&req)

	if me.Debug {
		log.Printf("Response: ENTRY_NOTIFY: %v", result)
	}
	return result
}
