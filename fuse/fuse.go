package fuse

import (
	"bytes"
	"encoding/binary"
	"expvar"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
)

// TODO make generic option setting.
const (
	// bufSize should be a power of two to minimize lossage in
	// BufferPool.
	bufSize = (1 << 16)
	maxRead = bufSize - PAGESIZE
)

type Empty interface{}

////////////////////////////////////////////////////////////////
// State related to this mount point.

// TODO - should gather stats and expose those for performance tuning.
type MountState struct {
	// We should store the RawFuseFile/Dirs on the Go side,
	// otherwise our files may be GCd.  Here, the index is the Fh
	// field

	openedFiles      map[uint64]RawFuseFile
	openedFilesMutex sync.RWMutex
	nextFreeFile     uint64

	openedDirs      map[uint64]RawFuseDir
	openedDirsMutex sync.RWMutex
	nextFreeDir     uint64

	// Empty if unmounted.
	mountPoint string
	fileSystem RawFileSystem

	// I/O with kernel and daemon.
	mountFile     *os.File
	errorChannel  chan os.Error
	outputChannel chan [][]byte

	// Run each operation in its own Go-routine.
	threaded bool

	// Dump debug info onto stdout.
	Debug bool

	// For efficient reads.
	buffers *BufferPool

	operationCounts *expvar.Map
}

func (me *MountState) RegisterFile(file RawFuseFile) uint64 {
	me.openedFilesMutex.Lock()
	defer me.openedFilesMutex.Unlock()
	// We will be screwed if nextFree ever wraps.
	me.nextFreeFile++
	index := me.nextFreeFile
	me.openedFiles[index] = file
	return index
}

func (me *MountState) FindFile(index uint64) RawFuseFile {
	me.openedFilesMutex.RLock()
	defer me.openedFilesMutex.RUnlock()
	return me.openedFiles[index]
}

func (me *MountState) UnregisterFile(handle uint64) {
	me.openedFilesMutex.Lock()
	defer me.openedFilesMutex.Unlock()
	me.openedFiles[handle] = nil, false
}

func (me *MountState) RegisterDir(dir RawFuseDir) uint64 {
	me.openedDirsMutex.Lock()
	defer me.openedDirsMutex.Unlock()
	me.nextFreeDir++
	index := me.nextFreeDir
	me.openedDirs[index] = dir
	return index
}

func (me *MountState) FindDir(index uint64) RawFuseDir {
	me.openedDirsMutex.RLock()
	defer me.openedDirsMutex.RUnlock()
	return me.openedDirs[index]
}

func (me *MountState) UnregisterDir(handle uint64) {
	me.openedDirsMutex.Lock()
	defer me.openedDirsMutex.Unlock()
	me.openedDirs[handle] = nil, false
}

// Mount filesystem on mountPoint.
//
// If threaded is set, each filesystem operation executes in a
// separate goroutine, and errors and writes are done asynchronously
// using channels.
//
// TODO - error handling should perhaps be user-serviceable.
func (me *MountState) Mount(mountPoint string) os.Error {
	file, mp, err := mount(mountPoint)
	if err != nil {
		return err
	}
	me.mountPoint = mp
	me.mountFile = file

	me.operationCounts = expvar.NewMap(fmt.Sprintf("mount(%v)", mountPoint))
	return nil
}

// Normally, callers should run loop() and wait for FUSE to exit, but
// tests will want to run this in a goroutine.
func (me *MountState) Loop(threaded bool) {
	me.threaded = threaded
	if me.threaded {
		me.outputChannel = make(chan [][]byte, 100)
		me.errorChannel = make(chan os.Error, 100)
		go me.asyncWriterThread()
		go me.DefaultErrorHandler()
	}

	me.loop()

	if me.threaded {
		close(me.outputChannel)
		close(me.errorChannel)
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

func (me *MountState) DefaultErrorHandler() {
	for err := range me.errorChannel {
		if err == os.EOF || err == nil {
			break
		}
		log.Println("error: ", err)
	}
}

func (me *MountState) Error(err os.Error) {
	// It is safe to do errors unthreaded, since the logger is thread-safe.
	if !me.threaded || me.Debug {
		log.Println("error: ", err)
	} else {
		me.errorChannel <- err
	}
}

func (me *MountState) Write(packet [][]byte) {
	if packet == nil {
		return
	}

	if me.threaded {
		me.outputChannel <- packet
	} else {
		me.syncWrite(packet)
	}
}

func NewMountState(fs RawFileSystem) *MountState {
	me := new(MountState)
	me.openedDirs = make(map[uint64]RawFuseDir)
	me.openedFiles = make(map[uint64]RawFuseFile)
	me.mountPoint = ""
	me.fileSystem = fs
	me.buffers = NewBufferPool()
	return me

}

// TODO - have more statistics.
func (me *MountState) Stats() string {
	var lines []string

	// TODO - bufferpool should use expvar.
	lines = append(lines,
		fmt.Sprintf("buffers: %v", me.buffers.String()))

	for v := range expvar.Iter() {
		if strings.HasPrefix(v.Key, "mount") {
			lines = append(lines, fmt.Sprintf("%v: %v\n", v.Key, v.Value))
		}
	}
	return strings.Join(lines, "\n")
}

////////////////
// Private routines.

func (me *MountState) asyncWriterThread() {
	for packet := range me.outputChannel {
		me.syncWrite(packet)
	}
}

func (me *MountState) syncWrite(packet [][]byte) {
	_, err := Writev(me.mountFile.Fd(), packet)
	if err != nil {
		me.Error(os.NewError(fmt.Sprintf("writer: Writev %v failed, err: %v", packet, err)))
	}
	for _, v := range packet {
		me.buffers.FreeBuffer(v)
	}
}


////////////////////////////////////////////////////////////////
// Logic for the control loop.

func (me *MountState) loop() {
	// See fuse_kern_chan_receive()
	for {
		buf := me.buffers.AllocBuffer(bufSize)
		n, err := me.mountFile.Read(buf)
		if err != nil {
			errNo := OsErrorToFuseError(err)

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

			readErr := os.NewError(fmt.Sprintf("Failed to read from fuse conn: %v", err))
			me.Error(readErr)
			break
		}

		if me.threaded {
			go me.handle(buf[0:n])
		} else {
			me.handle(buf[0:n])
		}
	}

	me.mountFile.Close()
}

func (me *MountState) handle(in_data []byte) {
	r := bytes.NewBuffer(in_data)
	header := new(InHeader)
	err := binary.Read(r, binary.LittleEndian, header)
	if err == os.EOF {
		err = os.NewError(fmt.Sprintf("MountPoint, handle: can't read a header, in_data: %v", in_data))
	}
	if err != nil {
		me.Error(err)
		return
	}
	me.Write(dispatch(me, header, r))
	me.buffers.FreeBuffer(in_data)
}


func dispatch(state *MountState, h *InHeader, arg *bytes.Buffer) (outBytes [][]byte) {
	// TODO - would be nice to remove this logging from the critical path.
	state.operationCounts.Add(operationName(h.Opcode), 1)

	input := newInput(h.Opcode)
	if input != nil && !parseLittleEndian(arg, input) {
		return serialize(h, EIO, nil, nil, false)
	}

	var out Empty
	var status Status
	var flatData []byte

	out = nil
	status = OK
	fs := state.fileSystem

	filename := ""
	// Perhaps a map is faster?
	if h.Opcode == FUSE_UNLINK || h.Opcode == FUSE_RMDIR ||
		h.Opcode == FUSE_LOOKUP || h.Opcode == FUSE_MKDIR ||
		h.Opcode == FUSE_MKNOD || h.Opcode == FUSE_CREATE ||
		h.Opcode == FUSE_LINK {
		filename = strings.TrimRight(string(arg.Bytes()), "\x00")
	}

	if state.Debug {
		nm := ""
		if filename != "" {
			nm = "n: '" + filename + "'"
		}
		log.Printf("Dispatch: %v, NodeId: %v %s\n", operationName(h.Opcode), h.NodeId, nm)
	}

	// Follow ordering of fuse_lowlevel.h.
	switch h.Opcode {
	case FUSE_INIT:
		out, status = initFuse(state, h, input.(*InitIn))
	case FUSE_DESTROY:
		fs.Destroy(h, input.(*InitIn))
	case FUSE_LOOKUP:
		out, status = fs.Lookup(h, filename)
	case FUSE_FORGET:
		fs.Forget(h, input.(*ForgetIn))
		// If we try to write OK, nil, we will get
		// error:  writer: Writev [[16 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0]]
		// failed, err: writev: no such file or directory
		return nil
	case FUSE_GETATTR:
		// TODO - if input.Fh is set, do file.GetAttr
		out, status = fs.GetAttr(h, input.(*GetAttrIn))
	case FUSE_SETATTR:
		out, status = doSetattr(state, h, input.(*SetAttrIn))
	case FUSE_READLINK:
		out, status = fs.Readlink(h)
	case FUSE_MKNOD:
		out, status = fs.Mknod(h, input.(*MknodIn), filename)
	case FUSE_MKDIR:
		out, status = fs.Mkdir(h, input.(*MkdirIn), filename)
	case FUSE_UNLINK:
		status = fs.Unlink(h, filename)
	case FUSE_RMDIR:
		status = fs.Rmdir(h, filename)
	case FUSE_SYMLINK:
		filenames := strings.Split(string(arg.Bytes()), "\x00", 3)
		if len(filenames) >= 2 {
			out, status = fs.Symlink(h, filenames[1], filenames[0])
		} else {
			status = EIO
		}
	case FUSE_RENAME:
		filenames := strings.Split(string(arg.Bytes()), "\x00", 3)
		if len(filenames) >= 2 {
			status = fs.Rename(h, input.(*RenameIn), filenames[0], filenames[1])
		} else {
			status = EIO
		}
	case FUSE_LINK:
		out, status = fs.Link(h, input.(*LinkIn), filename)
	case FUSE_OPEN:
		out, status = doOpen(state, h, input.(*OpenIn))
	case FUSE_READ:
		flatData, status = doRead(state, h, input.(*ReadIn), state.buffers)
	case FUSE_WRITE:
		out, status = doWrite(state, h, input.(*WriteIn), arg.Bytes())
	case FUSE_FLUSH:
		out, status = doFlush(state, h, input.(*FlushIn))
	case FUSE_RELEASE:
		out, status = doRelease(state, h, input.(*ReleaseIn))
	case FUSE_FSYNC:
		status = doFsync(state, h, input.(*FsyncIn))
	case FUSE_OPENDIR:
		out, status = doOpenDir(state, h, input.(*OpenIn))
	case FUSE_READDIR:
		out, status = doReadDir(state, h, input.(*ReadIn))
	case FUSE_RELEASEDIR:
		out, status = doReleaseDir(state, h, input.(*ReleaseIn))
	case FUSE_FSYNCDIR:
		// todo- check input type.
		status = doFsyncDir(state, h, input.(*FsyncIn))

	// TODO - implement XAttr routines.
	// case FUSE_SETXATTR:
	//	status = fs.SetXAttr(h, input.(*SetXAttrIn))
	// case FUSE_GETXATTR:
	//	out, status = fs.GetXAttr(h, input.(*GetXAttrIn))
	// case FUSE_LISTXATTR:
	// case FUSE_REMOVEXATTR

	case FUSE_ACCESS:
		status = fs.Access(h, input.(*AccessIn))
	case FUSE_CREATE:
		out, status = doCreate(state, h, input.(*CreateIn), filename)

	// TODO - implement file locking.
	// case FUSE_SETLK
	// case FUSE_SETLKW
	case FUSE_BMAP:
		out, status = fs.Bmap(h, input.(*BmapIn))
	case FUSE_IOCTL:
		out, status = fs.Ioctl(h, input.(*IoctlIn))
	case FUSE_POLL:
		out, status = fs.Poll(h, input.(*PollIn))
	// TODO - figure out how to support this
	// case FUSE_INTERRUPT
	default:
		state.Error(os.NewError(fmt.Sprintf("Unsupported OpCode: %d=%v", h.Opcode, operationName(h.Opcode))))
		return serialize(h, ENOSYS, nil, nil, false)
	}

	return serialize(h, status, out, flatData, state.Debug)
}

func serialize(h *InHeader, res Status, out interface{}, flatData []byte, debug bool) [][]byte {
	out_data := make([]byte, 0)
	b := new(bytes.Buffer)
	if out != nil && res == OK {
		err := binary.Write(b, binary.LittleEndian, out)
		if err == nil {
			out_data = b.Bytes()
		} else {
			panic(fmt.Sprintf("Can't serialize out: %v, err: %v", out, err))
		}
	}

	var hout OutHeader
	hout.Unique = h.Unique
	hout.Status = -res
	hout.Length = uint32(len(out_data) + SizeOfOutHeader + len(flatData))
	b = new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, &hout)
	if err != nil {
		panic("Can't serialize OutHeader")
	}

	data := [][]byte{b.Bytes(), out_data, flatData}

	if debug {
		val := fmt.Sprintf("%v", out)
		max := 1024
		if len(val) > max {
			val = val[:max] + fmt.Sprintf(" ...trimmed (response size %d)", hout.Length)
		}

		log.Printf("Serialize: %v code: %v value: %v flat: %d\n",
			operationName(h.Opcode), res, val, len(flatData))
	}

	return data
}

func initFuse(state *MountState, h *InHeader, input *InitIn) (Empty, Status) {
	out, initStatus := state.fileSystem.Init(h, input)
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

	return out, OK
}

////////////////////////////////////////////////////////////////
// Handling files.

func doOpen(state *MountState, header *InHeader, input *OpenIn) (genericOut Empty, code Status) {
	flags, fuseFile, status := state.fileSystem.Open(header, input)
	if status != OK {
		return nil, status
	}
	if fuseFile == nil {
		fmt.Println("fuseFile should not be nil.")
	}
	out := new(OpenOut)
	out.Fh = state.RegisterFile(fuseFile)
	out.OpenFlags = flags
	return out, status
}

func doCreate(state *MountState, header *InHeader, input *CreateIn, name string) (genericOut Empty, code Status) {
	flags, fuseFile, entry, status := state.fileSystem.Create(header, input, name)
	if status != OK {
		return nil, status
	}
	if fuseFile == nil {
		fmt.Println("fuseFile should not be nil.")
	}
	out := new(CreateOut)
	out.Entry = *entry
	out.Open.Fh = state.RegisterFile(fuseFile)
	out.Open.OpenFlags = flags
	return out, status
}

func doRelease(state *MountState, header *InHeader, input *ReleaseIn) (out Empty, code Status) {
	f := state.FindFile(input.Fh)
	state.fileSystem.Release(header, f)
	f.Release()
	state.UnregisterFile(input.Fh)
	return nil, OK
}

func doRead(state *MountState, header *InHeader, input *ReadIn, buffers *BufferPool) (out []byte, code Status) {
	output, code := state.FindFile(input.Fh).Read(input, buffers)
	return output, code
}

func doWrite(state *MountState, header *InHeader, input *WriteIn, data []byte) (out WriteOut, code Status) {
	n, status := state.FindFile(input.Fh).Write(input, data)
	out.Size = n
	return out, status
}

func doFsync(state *MountState, header *InHeader, input *FsyncIn) (code Status) {
	return state.FindFile(input.Fh).Fsync(input)
}

func doFlush(state *MountState, header *InHeader, input *FlushIn) (out Empty, code Status) {
	return nil, state.FindFile(input.Fh).Flush()
}

func doSetattr(state *MountState, header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	// TODO - if Fh != 0, we should do a FSetAttr instead.
	return state.fileSystem.SetAttr(header, input)
}

////////////////////////////////////////////////////////////////
// Handling directories

func doReleaseDir(state *MountState, header *InHeader, input *ReleaseIn) (out Empty, code Status) {
	d := state.FindDir(input.Fh)
	state.fileSystem.ReleaseDir(header, d)
	d.ReleaseDir()
	state.UnregisterDir(input.Fh)
	return nil, OK
}

func doOpenDir(state *MountState, header *InHeader, input *OpenIn) (genericOut Empty, code Status) {
	flags, fuseDir, status := state.fileSystem.OpenDir(header, input)
	if status != OK {
		return nil, status
	}

	out := new(OpenOut)
	out.Fh = state.RegisterDir(fuseDir)
	out.OpenFlags = flags
	return out, status
}

func doReadDir(state *MountState, header *InHeader, input *ReadIn) (out Empty, code Status) {
	dir := state.FindDir(input.Fh)
	entries, code := dir.ReadDir(input)
	if entries == nil {
		var emptyBytes []byte
		return emptyBytes, code
	}
	return entries.Bytes(), code
}

func doFsyncDir(state *MountState, header *InHeader, input *FsyncIn) (code Status) {
	return state.FindDir(input.Fh).FsyncDir(input)
}
