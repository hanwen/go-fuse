// Code that handles the control loop, and en/decoding messages
// to/from the kernel.  Dispatches calls into RawFileSystem.

package fuse

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// TODO make generic option setting.
const (
	// bufSize should be a power of two to minimize lossage in
	// BufferPool.
	bufSize = (1 << 16)
	maxRead = bufSize - PAGESIZE
)

////////////////////////////////////////////////////////////////
// State related to this mount point.

type request struct {
	inputBuf []byte

	// These split up inputBuf.
	inHeader *InHeader	// generic header
	inData   unsafe.Pointer	// per op data
	arg      []byte		// flat data.

	// Unstructured data, a pointer to the relevant XxxxOut struct.
	data     unsafe.Pointer
	status   Status
	flatData []byte

	// Header + structured data for what we send back to the kernel.
	// May be followed by flatData.
	outHeaderBytes []byte

	// Start timestamp for timing info.
	startNs    int64
	dispatchNs int64
	preWriteNs int64
}

func (me *request) filename() string {
	return strings.TrimRight(string(me.arg), "\x00")
}

func (me *request) filenames(count int) []string {
	return strings.Split(string(me.arg), "\x00", count)
}

type MountState struct {
	// Empty if unmounted.
	mountPoint string
	fileSystem RawFileSystem

	// I/O with kernel and daemon.
	mountFile *os.File

	// Dump debug info onto stdout.
	Debug bool

	// For efficient reads and writes.
	buffers *BufferPool

	statisticsMutex sync.Mutex
	operationCounts map[string]int64

	// In nanoseconds.
	operationLatencies map[string]int64
}

// Mount filesystem on mountPoint.
//
// TODO - error handling should perhaps be user-serviceable.
func (me *MountState) Mount(mountPoint string) os.Error {
	file, mp, err := mount(mountPoint)
	if err != nil {
		return err
	}
	me.mountPoint = mp
	me.mountFile = file

	me.operationCounts = make(map[string]int64)
	me.operationLatencies = make(map[string]int64)
	return nil
}

func (me *MountState) Unmount() os.Error {
	// Todo: flush/release all files/dirs?
	result := unmount(me.mountPoint)
	if result == nil {
		me.mountPoint = ""
	}
	return result
}

func (me *MountState) Error(err os.Error) {
	log.Println("error: ", err)
}

func (me *MountState) Write(req *request) {
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
		me.Error(os.NewError(fmt.Sprintf("writer: Write/Writev %v failed, err: %v. Opcode: %v",
			req.outHeaderBytes, err, operationName(req.inHeader.Opcode))))
	}
}

func NewMountState(fs RawFileSystem) *MountState {
	me := new(MountState)
	me.mountPoint = ""
	me.fileSystem = fs
	me.buffers = NewBufferPool()
	return me

}

func (me *MountState) Latencies() map[string]float64 {
	me.statisticsMutex.Lock()
	defer me.statisticsMutex.Unlock()

	r := make(map[string]float64)
	for k, v := range me.operationCounts {
		r[k] = 1e-6 * float64(me.operationLatencies[k]) / float64(v)
	}

	return r
}

func (me *MountState) OperationCounts() map[string]int64 {
	me.statisticsMutex.Lock()
	defer me.statisticsMutex.Unlock()

	r := make(map[string]int64)
	for k, v := range me.operationCounts {
		r[k] = v
	}
	return r
}

func (me *MountState) Stats() string {
	return fmt.Sprintf("buffer alloc count %d\nbuffers %v",
		me.buffers.AllocCount(), me.buffers.String())
}

////////////////////////////////////////////////////////////////
// Logic for the control loop.

func (me *MountState) newRequest() *request {
	req := new(request)
	req.status = OK
	req.inputBuf = me.buffers.AllocBuffer(bufSize)
	return req
}

func (me *MountState) readRequest(req *request) os.Error {
	n, err := me.mountFile.Read(req.inputBuf)
	// If we start timing before the read, we may take into
	// account waiting for input into the timing.
	req.startNs = time.Nanoseconds()
	req.inputBuf = req.inputBuf[0:n]
	return err
}

func (me *MountState) discardRequest(req *request) {
	endNs := time.Nanoseconds()
	dt := endNs - req.startNs

	me.statisticsMutex.Lock()
	defer me.statisticsMutex.Unlock()

	opname := operationName(req.inHeader.Opcode)
	key := opname
	me.operationCounts[key] += 1
	me.operationLatencies[key] += dt

	key += "-dispatch"
	me.operationLatencies[key] += (req.dispatchNs - req.startNs)
	me.operationCounts[key] += 1

	key = opname + "-write"
	me.operationLatencies[key] += (endNs - req.preWriteNs)
	me.operationCounts[key] += 1

	me.buffers.FreeBuffer(req.inputBuf)
	me.buffers.FreeBuffer(req.flatData)
}

// Normally, callers should run Loop() and wait for FUSE to exit, but
// tests will want to run this in a goroutine.
//
// If threaded is set, each filesystem operation executes in a
// separate goroutine.
func (me *MountState) Loop(threaded bool) {
	// See fuse_kern_chan_receive()
	for {
		req := me.newRequest()

		err := me.readRequest(req)
		if err != nil {
			errNo := OsErrorToErrno(err)

			// Retry.
			if errNo == syscall.ENOENT {
				me.discardRequest(req)
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

			readErr := os.NewError(fmt.Sprintf("Failed to read from fuse conn: %v", err))
			me.Error(readErr)
			break
		}

		if threaded {
			go me.handle(req)
		} else {
			me.handle(req)
		}
	}
	me.mountFile.Close()
}

func (me *MountState) handle(req *request) {
	defer me.discardRequest(req)
	req.dispatchNs = time.Nanoseconds()

	inHSize := unsafe.Sizeof(InHeader{})
	if len(req.inputBuf) < inHSize {
		me.Error(os.NewError(fmt.Sprintf("Short read for input header: %v", req.inputBuf)))
		return
	}

	req.inHeader = (*InHeader)(unsafe.Pointer(&req.inputBuf[0]))
	req.arg = req.inputBuf[inHSize:]
	me.dispatch(req)
	if req.inHeader.Opcode != FUSE_FORGET {
		serialize(req, me.Debug)
		req.preWriteNs = time.Nanoseconds()
		me.Write(req)
	}
}

func (me *MountState) dispatch(req *request) {
	h := req.inHeader
	argSize, ok := inputSize(h.Opcode)
	if !ok {
		log.Println("Unknown opcode %d (input)", h.Opcode)
		req.status = ENOSYS
		return
	}
	
	if len(req.arg) < argSize {
		log.Println("Short read for %v: %v", h.Opcode, req.arg)
		req.status = EIO
		return
	}

	if argSize > 0 {
		req.inData = unsafe.Pointer(&req.arg[0])
		req.arg = req.arg[argSize:]
	}

	f := lookupOperation(h.Opcode)
	if f != nil {
		f(me, req)
		return
	}
	
	req.status = OK

	filename := ""
	// Perhaps a map is faster?
	if h.Opcode == FUSE_UNLINK || h.Opcode == FUSE_RMDIR ||
		h.Opcode == FUSE_LOOKUP || h.Opcode == FUSE_MKDIR ||
		h.Opcode == FUSE_MKNOD || 
		h.Opcode == FUSE_LINK ||
		h.Opcode == FUSE_REMOVEXATTR {
		filename = strings.TrimRight(string(req.arg), "\x00")
	}
	if me.Debug {
		nm := ""
		if filename != "" {
			nm = "n: '" + filename + "'"
		}
		if h.Opcode == FUSE_RENAME {
			nm = "n: '" + string(req.arg) + "'"
		}

		log.Printf("Dispatch: %v, NodeId: %v %s\n", operationName(h.Opcode), h.NodeId, nm)
	}

	// Follow ordering of fuse_lowlevel.h.
	var status Status
	fs := me.fileSystem
	switch h.Opcode {
	case FUSE_INIT:
		req.data, status = me.init(h, (*InitIn)(req.inData))
	case FUSE_DESTROY:
		fs.Destroy(h, (*InitIn)(req.inData))
	case FUSE_LOOKUP:
		lookupOut, s := fs.Lookup(h, filename)
		status = s
		req.data = unsafe.Pointer(lookupOut)
	case FUSE_FORGET:
		fs.Forget(h, (*ForgetIn)(req.inData))
		// If we try to write OK, nil, we will get
		// error:  writer: Writev [[16 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0]]
		// failed, err: writev: no such file or directory
		return
	case FUSE_GETATTR:
		// TODO - if req.inData.Fh is set, do file.GetAttr
		attrOut, s := fs.GetAttr(h, (*GetAttrIn)(req.inData))
		status = s
		req.data = unsafe.Pointer(attrOut)
	case FUSE_READLINK:
		req.flatData, status = fs.Readlink(h)
	case FUSE_MKNOD:
		entryOut, s := fs.Mknod(h, (*MknodIn)(req.inData), filename)
		status = s
		req.data = unsafe.Pointer(entryOut)
	case FUSE_MKDIR:
		entryOut, s := fs.Mkdir(h, (*MkdirIn)(req.inData), filename)
		status = s
		req.data = unsafe.Pointer(entryOut)
	case FUSE_UNLINK:
		status = fs.Unlink(h, filename)
	case FUSE_RMDIR:
		status = fs.Rmdir(h, filename)
	case FUSE_SYMLINK:
		filenames := req.filenames(3)
		if len(filenames) >= 2 {
			entryOut, s := fs.Symlink(h, filenames[1], filenames[0])
			status = s
			req.data = unsafe.Pointer(entryOut)
		} else {
			status = EIO
		}
	case FUSE_RENAME:
		filenames := strings.Split(string(req.arg), "\x00", 3)
		if len(filenames) >= 2 {
			status = fs.Rename(h, (*RenameIn)(req.inData), filenames[0], filenames[1])
		} else {
			status = EIO
		}
	case FUSE_LINK:
		entryOut, s := fs.Link(h, (*LinkIn)(req.inData), filename)
		status = s
		req.data = unsafe.Pointer(entryOut)
	case FUSE_READ:
		req.flatData, status = me.fileSystem.Read((*ReadIn)(req.inData), me.buffers)
	case FUSE_FLUSH:
		status = me.fileSystem.Flush((*FlushIn)(req.inData))
	case FUSE_RELEASE:
		me.fileSystem.Release(h, (*ReleaseIn)(req.inData))
	case FUSE_FSYNC:
		status = me.fileSystem.Fsync((*FsyncIn)(req.inData))
	case FUSE_RELEASEDIR:
		me.fileSystem.ReleaseDir(h, (*ReleaseIn)(req.inData))
	case FUSE_FSYNCDIR:
		status = me.fileSystem.FsyncDir(h, (*FsyncIn)(req.inData))
	case FUSE_SETXATTR:
		splits := bytes.Split(req.arg, []byte{0}, 2)
		status = fs.SetXAttr(h, (*SetXAttrIn)(req.inData), string(splits[0]), splits[1])
	case FUSE_REMOVEXATTR:
		status = fs.RemoveXAttr(h, filename)
	case FUSE_ACCESS:
		status = fs.Access(h, (*AccessIn)(req.inData))
	// TODO - implement file locking.
	// case FUSE_SETLK
	// case FUSE_SETLKW
	case FUSE_BMAP:
		bmapOut, s := fs.Bmap(h, (*BmapIn)(req.inData))
		status = s
		req.data = unsafe.Pointer(bmapOut)
	case FUSE_IOCTL:
		ioctlOut, s := fs.Ioctl(h, (*IoctlIn)(req.inData))
		status = s
		req.data = unsafe.Pointer(ioctlOut)
	case FUSE_POLL:
		pollOut, s := fs.Poll(h, (*PollIn)(req.inData))
		status = s
		req.data = unsafe.Pointer(pollOut)

	// TODO - figure out how to support this
	// case FUSE_INTERRUPT
	default:
		me.Error(os.NewError(fmt.Sprintf("Unsupported OpCode: %d=%v", h.Opcode, operationName(h.Opcode))))
		req.status = ENOSYS
		return
	}

	req.status = status
}

// Thanks to Andrew Gerrand for this hack.
func asSlice(ptr unsafe.Pointer, byteCount int) []byte {
	h := &reflect.SliceHeader{uintptr(ptr), byteCount, byteCount}
	return *(*[]byte)(unsafe.Pointer(h))
}

func serialize(req *request, debug bool) {
	dataLength, ok := outputSize(req.inHeader.Opcode)
	if !ok {
		log.Println("Unknown opcode %d (output)", req.inHeader.Opcode)
		req.status = ENOSYS
		return
	}
	if req.data == nil || req.status != OK {
		dataLength = 0
	}

	sizeOfOutHeader := unsafe.Sizeof(OutHeader{})

	req.outHeaderBytes = make([]byte, sizeOfOutHeader+dataLength)
	outHeader := (*OutHeader)(unsafe.Pointer(&req.outHeaderBytes[0]))
	outHeader.Unique = req.inHeader.Unique
	outHeader.Status = -req.status
	outHeader.Length = uint32(sizeOfOutHeader + dataLength + len(req.flatData))

	copy(req.outHeaderBytes[sizeOfOutHeader:], asSlice(req.data, dataLength))
	if debug {
		val := fmt.Sprintf("%v", replyString(req.inHeader.Opcode, req.data))
		max := 1024
		if len(val) > max {
			val = val[:max] + fmt.Sprintf(" ...trimmed (response size %d)", outHeader.Length)
		}

		msg := ""
		if len(req.flatData) > 0 {
			msg = fmt.Sprintf(" flat: %d\n", len(req.flatData))
		}
		log.Printf("Serialize: %v code: %v value: %v%v",
			operationName(req.inHeader.Opcode), req.status, val, msg)
	}
}

func (me *MountState) init(h *InHeader, input *InitIn) (unsafe.Pointer, Status) {
	out, initStatus := me.fileSystem.Init(h, input)
	if initStatus != OK {
		return nil, initStatus
	}

	if input.Major != FUSE_KERNEL_VERSION {
		fmt.Printf("Major versions does not match. Given %d, want %d\n", input.Major, FUSE_KERNEL_VERSION)
		return nil, EIO
	}
	if input.Minor < FUSE_KERNEL_MINOR_VERSION {
		fmt.Printf("Minor version is less than we support. Given %d, want at least %d\n", input.Minor, FUSE_KERNEL_MINOR_VERSION)
		return nil, EIO
	}

	out.Major = FUSE_KERNEL_VERSION
	out.Minor = FUSE_KERNEL_MINOR_VERSION
	out.MaxReadAhead = input.MaxReadAhead
	out.Flags = FUSE_ASYNC_READ | FUSE_POSIX_LOCKS | FUSE_BIG_WRITES

	out.MaxWrite = maxRead

	return unsafe.Pointer(out), OK
}
