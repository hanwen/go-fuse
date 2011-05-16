package fuse

import (
	"log"
	"os"
	"syscall"
	"time"
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

	optStr := ""
	if opts != nil && opts.AllowOther {
		optStr = "allow_other"
	}
	
	file, mp, err := mount(mountPoint, optStr)
	if err != nil {
		return err
	}
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
	result := unmount(me.mountPoint)
	if result == nil {
		me.mountPoint = ""
	}
	return result
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

func (me *MountState) newRequest(oldReq *request) *request {
	if oldReq != nil {
		me.buffers.FreeBuffer(oldReq.flatData)

		*oldReq = request{
			status:   OK,
			inputBuf: oldReq.inputBuf[0:bufSize],
		}
		return oldReq
	}

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
// If threaded is given, each filesystem operation executes in a
// separate goroutine.
func (me *MountState) Loop(threaded bool) {
	// To limit scheduling overhead, we spawn multiple read loops.
	// This means that the request once read does not need to be
	// assigned to another thread, so it avoids a context switch.
	if threaded {
		for i := 0; i < _BACKGROUND_TASKS; i++ {
			go me.loop()
		}
	}
	me.loop()
	me.mountFile.Close()
}

func (me *MountState) loop() {
	var lastReq *request
	for {
		req := me.newRequest(lastReq)
		lastReq = req
		err := me.readRequest(req)
		if err != nil {
			errNo := OsErrorToErrno(err)

			// Retry.
			if errNo == syscall.ENOENT {
				continue
			}

			// According to fuse_chan_receive()
			if errNo == syscall.ENODEV {
				break
			}

			// What I see on linux-x86 2.6.35.10.
			if errNo == syscall.ENOSYS {
				break
			}

			log.Printf("Failed to read from fuse conn: %v", err)
			break
		}
		me.handleRequest(req)
	}
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

	me.write(req)
}

func (me *MountState) write(req *request) {
	// If we try to write OK, nil, we will get
	// error:  writer: Writev [[16 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0]]
	// failed, err: writev: no such file or directory
	if req.inHeader.opcode == _OP_FORGET {
		return
	}

	req.serialize()
	if me.Debug {
		log.Println(req.OutputDebug())
	}

	if me.LatencyMap != nil {
		req.preWriteNs = time.Nanoseconds()
	}

	if req.outHeaderBytes == nil {
		return
	}

	var err os.Error
	if req.flatData == nil {
		_, err = me.mountFile.Write(req.outHeaderBytes)
	} else {
		_, err = Writev(me.mountFile.Fd(),
			[][]byte{req.outHeaderBytes, req.flatData})
	}

	if err != nil {
		log.Printf("writer: Write/Writev %v failed, err: %v. opcode: %v",
			req.outHeaderBytes, err, operationName(req.inHeader.opcode))
	}
}
