package fuse

import (
	"log"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
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

	// For efficient reads and writes.
	buffers *BufferPoolImpl

	latencies *LatencyMap

	opts           *MountOptions
	kernelSettings raw.InitIn
}

func (me *MountState) KernelSettings() raw.InitIn {
	return me.kernelSettings
}

func (me *MountState) MountPoint() string {
	return me.mountPoint
}

// Mount filesystem on mountPoint.
func (me *MountState) Mount(mountPoint string, opts *MountOptions) error {
	if opts == nil {
		opts = &MountOptions{
			MaxBackground: _DEFAULT_BACKGROUND_TASKS,
		}
	}
	o := *opts
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
	me.opts = &o

	optStrs := opts.Options
	if opts.AllowOther {
		optStrs = append(optStrs, "allow_other")
	}

	file, mp, err := mount(mountPoint, strings.Join(optStrs, ","))
	if err != nil {
		return err
	}
	initParams := RawFsInit{
		InodeNotify: func(n *raw.NotifyInvalInodeOut) Status {
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
		me.latencies = NewLatencyMap()
	} else {
		me.latencies = nil
	}
}

func (me *MountState) Unmount() (err error) {
	if me.mountPoint == "" {
		return nil
	}
	delay := time.Duration(0)
	for try := 0; try < 5; try++ {
		err = unmount(me.mountPoint)
		if err == nil {
			break
		}

		// Sleep for a bit. This is not pretty, but there is
		// no way we can be certain that the kernel thinks all
		// open files have already been closed.
		delay = 2*delay + 5*time.Millisecond
		time.Sleep(delay)
	}
	me.mountPoint = ""
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
	if me.latencies == nil {
		return nil
	}
	return me.latencies.Latencies(1e-3)
}

func (me *MountState) OperationCounts() map[string]int {
	if me.latencies == nil {
		return nil
	}
	return me.latencies.Counts()
}

func (me *MountState) BufferPoolStats() string {
	return me.buffers.String()
}

func (me *MountState) newRequest() *request {
	r := &request{
		pool:               me.buffers,
	}
	return r
}

func (me *MountState) recordStats(req *request) {
	if me.latencies != nil {
		endNs := time.Now().UnixNano()
		dt := endNs - req.startNs

		opname := operationName(req.inHeader.Opcode)
		me.latencies.AddMany(
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
	me.mountFile = nil
}

func (me *MountState) loop() {
	var dest []byte
	for {
		if dest == nil {
			dest = me.buffers.AllocBuffer(uint32(me.opts.MaxWrite + 4096))
		}
		
		n, err := me.mountFile.Read(dest)
		if err != nil {
			errNo := ToStatus(err)
		
			// Retry.
			if errNo == ENOENT {
				continue
			}

			if errNo == ENODEV {
				// Unmount.
				break
			}

			log.Printf("Failed to read from fuse conn: %v", errNo)
			break
		}
		
		req := me.newRequest()
		if me.latencies != nil {
			req.startNs = time.Now().UnixNano()
		}
		if req.setInput(dest[:n]) {
			dest = nil
		}

		// When closely analyzing timings, the context switch
		// generates some delay.  While unfortunate, the
		// alternative is to have a fixed goroutine pool,
		// which will lock up the FS if the daemon has too
		// many blocking calls.
		go func(r *request) {
			me.handleRequest(r)
			r.Discard()
		}(req)
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
		log.Printf("Unimplemented opcode %v", operationName(req.inHeader.Opcode))
		req.status = ENOSYS
	}

	if req.status.Ok() {
		req.handler.Func(me, req)
	}

	errNo := me.write(req)
	if errNo != 0 {
		log.Printf("writer: Write/Writev %v failed, err: %v. opcode: %v",
			req.outHeaderBytes, errNo, operationName(req.inHeader.Opcode))
	}
}

func (me *MountState) write(req *request) Status {
	// Forget does not wait for reply.
	if req.inHeader.Opcode == _OP_FORGET || req.inHeader.Opcode == _OP_BATCH_FORGET {
		return OK
	}

	req.serialize()
	if me.Debug {
		log.Println(req.OutputDebug())
	}

	if me.latencies != nil {
		req.preWriteNs = time.Now().UnixNano()
	}

	if req.outHeaderBytes == nil {
		return OK
	}

	var err error
	if req.flatData == nil {
		_, err = me.mountFile.Write(req.outHeaderBytes)
	} else {
		_, err = Writev(int(me.mountFile.Fd()),
			[][]byte{req.outHeaderBytes, req.flatData})
	}

	return ToStatus(err)
}

func (me *MountState) writeInodeNotify(entry *raw.NotifyInvalInodeOut) Status {
	req := request{
		inHeader: &raw.InHeader{
			Opcode: _OP_NOTIFY_INODE,
		},
		handler: operationHandlers[_OP_NOTIFY_INODE],
		status:  raw.NOTIFY_INVAL_INODE,
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
