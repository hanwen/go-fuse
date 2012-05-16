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

func (ms *MountState) KernelSettings() raw.InitIn {
	return ms.kernelSettings
}

func (ms *MountState) MountPoint() string {
	return ms.mountPoint
}

// Mount filesystem on mountPoint.
func (ms *MountState) Mount(mountPoint string, opts *MountOptions) error {
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
	ms.opts = &o

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
	ms.mountPoint = ""
	return err
}

func NewMountState(fs RawFileSystem) *MountState {
	ms := new(MountState)
	ms.mountPoint = ""
	ms.fileSystem = fs
	ms.buffers = NewBufferPool()
	return ms
}

func (ms *MountState) Latencies() map[string]float64 {
	if ms.latencies == nil {
		return nil
	}
	return ms.latencies.Latencies(1e-3)
}

func (ms *MountState) OperationCounts() map[string]int {
	if ms.latencies == nil {
		return nil
	}
	return ms.latencies.Counts()
}

func (ms *MountState) BufferPoolStats() string {
	return ms.buffers.String()
}

func (ms *MountState) newRequest() *request {
	r := &request{
		pool:               ms.buffers,
	}
	return r
}

func (ms *MountState) recordStats(req *request) {
	if ms.latencies != nil {
		endNs := time.Now().UnixNano()
		dt := endNs - req.startNs

		opname := operationName(req.inHeader.Opcode)
		ms.latencies.AddMany(
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
func (ms *MountState) Loop() {
	ms.loop()
	ms.mountFile.Close()
	ms.mountFile = nil
}

func (ms *MountState) loop() {
	var dest []byte
	for {
		if dest == nil {
			dest = ms.buffers.AllocBuffer(uint32(ms.opts.MaxWrite + 4096))
		}
		
		n, err := ms.mountFile.Read(dest)
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
		
		req := ms.newRequest()
		if ms.latencies != nil {
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
			ms.handleRequest(r)
			r.Discard()
		}(req)
	}
}

func (ms *MountState) handleRequest(req *request) {
	defer ms.recordStats(req)

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
}

func (ms *MountState) write(req *request) Status {
	// Forget does not wait for reply.
	if req.inHeader.Opcode == _OP_FORGET || req.inHeader.Opcode == _OP_BATCH_FORGET {
		return OK
	}

	header, data := req.serialize()
	if ms.Debug {
		log.Println(req.OutputDebug())
	}

	if ms.latencies != nil {
		req.preWriteNs = time.Now().UnixNano()
	}

	if header == nil {
		return OK
	}
	var err error
	if data == nil {
		_, err = ms.mountFile.Write(header)
	} else {
		_, err = Writev(int(ms.mountFile.Fd()), [][]byte{header, data})
	}

	return ToStatus(err)
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
	nameBytes := []byte(name + "\000")
	req.outData = unsafe.Pointer(entry)
	req.flatData = nameBytes
	result := ms.write(&req)

	if ms.Debug {
		log.Printf("Response: ENTRY_NOTIFY: %v", result)
	}
	return result
}
