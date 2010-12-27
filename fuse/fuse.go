package fuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
)

const (
	bufSize = 66000
)

type Empty interface {

}

////////////////////////////////////////////////////////////////
// State related to this mount point.

type MountState struct {
	// We should store the RawFuseFile/Dirs on the Go side,
	// otherwise our files may be GCd.  Here, the index is the Fh
	// field

	openedFiles map[uint64] RawFuseFile
	openedFilesMutex sync.RWMutex
	nextFreeFile uint64

	openedDirs map[uint64] RawFuseDir
	openedDirsMutex sync.RWMutex
	nextFreeDir uint64

	// Empty if unmounted.
	mountPoint string
	fileSystem RawFileSystem

	// I/O with kernel and daemon.
	mountFile *os.File
	errorChannel chan os.Error
	outputChannel chan [][]byte
	
	// Run each operation in its own Go-routine.
	threaded bool

	// Dump debug info onto stdout. 
	Debug bool
}

func (self *MountState) RegisterFile(file RawFuseFile) uint64 {
	self.openedFilesMutex.Lock()
	defer self.openedFilesMutex.Unlock()
	// We will be screwed if nextFree ever wraps.
	self.nextFreeFile++
	index := self.nextFreeFile
	self.openedFiles[index] = file
	return index
}

func (self *MountState) FindFile(index uint64) RawFuseFile {
	self.openedFilesMutex.RLock()
	defer self.openedFilesMutex.RUnlock()
	return self.openedFiles[index]
}

func (self *MountState) UnregisterFile(handle uint64) {
	self.openedFilesMutex.Lock()
	defer self.openedFilesMutex.Unlock()
	self.openedFiles[handle] = nil, false
}

func (self *MountState) RegisterDir(dir RawFuseDir) uint64 {
	self.openedDirsMutex.Lock()
	defer self.openedDirsMutex.Unlock()
	self.nextFreeDir++
	index := self.nextFreeDir
	self.openedDirs[index] = dir
	return index
}

func (self *MountState) FindDir(index uint64) RawFuseDir {
	self.openedDirsMutex.RLock()
	defer self.openedDirsMutex.RUnlock()
	return self.openedDirs[index]
}

func (self *MountState) UnregisterDir(handle uint64) {
	self.openedDirsMutex.Lock()
	defer self.openedDirsMutex.Unlock()
	self.openedDirs[handle] = nil, false
}

// Mount filesystem on mountPoint.
//
// If threaded is set, each filesystem operation executes in a
// separate goroutine, and errors and writes are done asynchronously
// using channels.
// 
// TODO - error handling should perhaps be user-serviceable.
func (self *MountState) Mount(mountPoint string, threaded bool) os.Error {
	file, mp, err := mount(mountPoint)
	if err != nil {
		return err
	}
	self.mountPoint = mp
	self.mountFile = file
	self.threaded = threaded
	
	if self.threaded {
		self.outputChannel = make(chan [][]byte, 100)
		self.errorChannel = make(chan os.Error, 100)
		go self.asyncWriterThread()
		go self.DefaultErrorHandler()
	}

	go self.loop()
	return nil
}

func (self *MountState) Unmount() os.Error {
	// Todo: flush/release all files/dirs?
	result := unmount(self.mountPoint)
	if result == nil {
		self.mountPoint = ""
	}
	return result
}

func (self *MountState) DefaultErrorHandler() {
	for err := range self.errorChannel {
		if err == os.EOF {
			break
		}
		log.Println("error: ", err)
	}
}

func (self *MountState) Error(err os.Error) {
	// It is safe to do errors unthreaded, since the logger is thread-safe.
	if self.Debug || self.threaded {
		log.Println("error: ", err)
	} else {
		self.errorChannel <- err
	}
}

func (self *MountState) Write(packet [][]byte) {
	if packet == nil {
		return
	}
	
	if self.threaded {
		self.outputChannel <- packet 
	} else {
		self.syncWrite(packet)
	}
}

func NewMountState(fs RawFileSystem) *MountState {
	self := new(MountState)
	self.openedDirs = make(map[uint64] RawFuseDir)
	self.openedFiles = make(map[uint64] RawFuseFile)
	self.mountPoint = ""
	self.fileSystem = fs
	return self
}

////////////////
// Private routines.

func (self *MountState) asyncWriterThread() {
	for packet := range self.outputChannel {
		self.syncWrite(packet)
	}
}

func (self *MountState) syncWrite(packet [][]byte) {
	_, err := Writev(self.mountFile.Fd(), packet)
	if err != nil {
		self.Error(os.NewError(fmt.Sprintf("writer: Writev %v failed, err: %v", packet, err)))
	}
}

////////////////////////////////////////////////////////////////
// Logic for the control loop.

func (self *MountState) loop() {
	buf := make([]byte, bufSize)

	// See fuse_kern_chan_receive()
	for {
		n, err := self.mountFile.Read(buf)
		if err != nil {
			errNo := OsErrorToFuseError(err)

			// Retry.
			if errNo == syscall.ENOENT {
				continue
			}

			// According to fuse_chan_receive()
			if errNo == syscall.ENODEV {
				break;
			}

			// What I see on linux-x86 2.6.35.10.
			if errNo == syscall.ENOSYS {
				break;
			}
			
			readErr := os.NewError(fmt.Sprintf("Failed to read from fuse conn: %v", err))
			self.Error(readErr)
			break
		}

		if self.threaded && !self.Debug {
			go self.handle(buf[0:n])
		} else {
			self.handle(buf[0:n])
		}
	}

	self.mountFile.Close()
	if self.threaded {
		close(self.outputChannel)
		close(self.errorChannel)
	}
}

func (self *MountState) handle(in_data []byte) {
	r := bytes.NewBuffer(in_data)
	header := new(InHeader)
	err := binary.Read(r, binary.LittleEndian, header)
	if err == os.EOF {
		err = os.NewError(fmt.Sprintf("MountPoint, handle: can't read a header, in_data: %v", in_data))
	}
	if err != nil {
		self.Error(err)
		return
	}
	self.Write(dispatch(self, header, r))
}




func dispatch(state *MountState, h *InHeader, arg *bytes.Buffer) (outBytes [][]byte) {
	input := newInput(h.Opcode)
	if input != nil && !parseLittleEndian(arg, input) {
		return serialize(h, EIO, nil)
	}

	var out Empty
	var status Status

	out = nil
	status = OK
	fs := state.fileSystem

	filename := ""
	// Perhaps a map is faster?
	if (h.Opcode == FUSE_UNLINK || h.Opcode == FUSE_RMDIR ||
		h.Opcode == FUSE_LOOKUP || h.Opcode == FUSE_MKDIR ||
		h.Opcode == FUSE_MKNOD || h.Opcode == FUSE_CREATE ||
		h.Opcode == FUSE_LINK) {
		filename = strings.TrimRight(string(arg.Bytes()), "\x00")
	}

	if state.Debug {
		log.Printf("Dispatch: %v, NodeId: %v, n: %v\n", operationName(h.Opcode), h.NodeId, filename)
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
		out, status = doRead(state, h, input.(*ReadIn))
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
		state.Error(os.NewError(fmt.Sprintf("Unsupported OpCode: %d", h.Opcode)))
		return serialize(h, ENOSYS, nil)
	}

	if state.Debug {
		log.Printf("Serialize: %v code: %v value: %v\n",
			operationName(h.Opcode), errorString(status), out)
	}
	
	return serialize(h, status, out)
}

func serialize(h *InHeader, res Status, out interface{}) (data [][]byte) {
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
	hout.Length = uint32(len(out_data) + SizeOfOutHeader)
	b = new(bytes.Buffer)
	err := binary.Write(b, binary.LittleEndian, &hout)
	if err != nil {
		panic("Can't serialize OutHeader")
	}
	_, _ = b.Write(out_data)
	data = [][]byte{b.Bytes()}
	return data
}

func initFuse(state* MountState, h *InHeader, input *InitIn) (Empty, Status) {
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
	out.Flags = FUSE_ASYNC_READ | FUSE_POSIX_LOCKS
	out.MaxWrite = 65536

	return out, OK
}

////////////////////////////////////////////////////////////////
// Handling files.

func doOpen(state *MountState, header *InHeader, input *OpenIn) (genericOut Empty, code Status) {
	flags, fuseFile, status := state.fileSystem.Open(header, input);
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
	flags, fuseFile, entry, status := state.fileSystem.Create(header, input, name);
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
	state.FindFile(input.Fh).Release()
	state.UnregisterFile(input.Fh)
	return nil, OK
}

func doRead(state *MountState, header *InHeader, input *ReadIn) (out Empty, code Status) {
	output, code := state.FindFile(input.Fh).Read(input)
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
	state.FindDir(input.Fh).ReleaseDir()
	state.UnregisterDir(input.Fh)
	return nil, OK
}

func doOpenDir(state *MountState, header *InHeader, input *OpenIn) (genericOut Empty, code Status) {
	flags, fuseDir, status := state.fileSystem.OpenDir(header, input);
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

////////////////////////////////////////////////////////////////
// DentryList.

func (de *DEntryList) AddString(name string, inode uint64, mode uint32) {
	de.Add([]byte(name), inode, mode)
}

func (de *DEntryList) Add(name []byte, inode uint64, mode uint32) {
	de.offset++

	dirent := new(Dirent)
	dirent.Off = de.offset
	dirent.Ino = inode
	dirent.NameLen = uint32(len(name))
	dirent.Typ = (mode & 0170000) >> 12

	err := binary.Write(&de.buf, binary.LittleEndian, dirent)
	if err != nil {
		panic("Serialization of Dirent failed")
	}
	de.buf.Write([]byte(name))
	de.buf.WriteByte(0)
	n := (len(name) + 1) % 8 // padding
	if n != 0 {
		de.buf.Write(make([]byte, 8-n))
	}
}

func (de *DEntryList) Bytes() []byte {
	return de.buf.Bytes()
}
