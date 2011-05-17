package fuse

import (
	"bytes"
	"fmt"
	"log"
	"unsafe"
)

var _ = log.Printf
var _ = fmt.Printf

type opcode int

const (
	_OP_LOOKUP      = opcode(1)
	_OP_FORGET      = opcode(2)
	_OP_GETATTR     = opcode(3)
	_OP_SETATTR     = opcode(4)
	_OP_READLINK    = opcode(5)
	_OP_SYMLINK     = opcode(6)
	_OP_MKNOD       = opcode(8)
	_OP_MKDIR       = opcode(9)
	_OP_UNLINK      = opcode(10)
	_OP_RMDIR       = opcode(11)
	_OP_RENAME      = opcode(12)
	_OP_LINK        = opcode(13)
	_OP_OPEN        = opcode(14)
	_OP_READ        = opcode(15)
	_OP_WRITE       = opcode(16)
	_OP_STATFS      = opcode(17)
	_OP_RELEASE     = opcode(18)
	_OP_FSYNC       = opcode(20)
	_OP_SETXATTR    = opcode(21)
	_OP_GETXATTR    = opcode(22)
	_OP_LISTXATTR   = opcode(23)
	_OP_REMOVEXATTR = opcode(24)
	_OP_FLUSH       = opcode(25)
	_OP_INIT        = opcode(26)
	_OP_OPENDIR     = opcode(27)
	_OP_READDIR     = opcode(28)
	_OP_RELEASEDIR  = opcode(29)
	_OP_FSYNCDIR    = opcode(30)
	_OP_GETLK       = opcode(31)
	_OP_SETLK       = opcode(32)
	_OP_SETLKW      = opcode(33)
	_OP_ACCESS      = opcode(34)
	_OP_CREATE      = opcode(35)
	_OP_INTERRUPT   = opcode(36)
	_OP_BMAP        = opcode(37)
	_OP_DESTROY     = opcode(38)
	_OP_IOCTL       = opcode(39)
	_OP_POLL        = opcode(40)

	_OPCODE_COUNT = opcode(41)
)


////////////////////////////////////////////////////////////////

func doInit(state *MountState, req *request) {
	const (
		FUSE_KERNEL_VERSION       = 7
		FUSE_KERNEL_MINOR_VERSION = 13
	)

	input := (*InitIn)(req.inData)
	if input.Major != FUSE_KERNEL_VERSION {
		log.Printf("Major versions does not match. Given %d, want %d\n", input.Major, FUSE_KERNEL_VERSION)
		req.status = EIO
		return
	}
	if input.Minor < FUSE_KERNEL_MINOR_VERSION {
		log.Printf("Minor version is less than we support. Given %d, want at least %d\n", input.Minor, FUSE_KERNEL_MINOR_VERSION)
		req.status = EIO
		return
	}

	state.kernelSettings = *input
	state.kernelSettings.Flags = input.Flags & (CAP_ASYNC_READ | CAP_BIG_WRITES | CAP_FILE_OPS)
	out := &InitOut{
		Major:               FUSE_KERNEL_VERSION,
		Minor:               FUSE_KERNEL_MINOR_VERSION,
		MaxReadAhead:        input.MaxReadAhead,
		Flags:               state.kernelSettings.Flags,
		MaxWrite:            maxRead,
		CongestionThreshold: _BACKGROUND_TASKS * 3 / 4,
		MaxBackground:       _BACKGROUND_TASKS,
	}

	req.outData = unsafe.Pointer(out)
	req.status = OK
}

func doOpen(state *MountState, req *request) {
	flags, handle, status := state.fileSystem.Open(req.inHeader, (*OpenIn)(req.inData))
	req.status = status
	if status != OK {
		return
	}

	out := &OpenOut{
		Fh:        handle,
		OpenFlags: flags,
	}

	req.outData = unsafe.Pointer(out)
}

func doCreate(state *MountState, req *request) {
	flags, handle, entry, status := state.fileSystem.Create(req.inHeader, (*CreateIn)(req.inData), req.filenames[0])
	req.status = status
	if status.Ok() {
		req.outData = unsafe.Pointer(&CreateOut{
			EntryOut: *entry,
			OpenOut: OpenOut{
				Fh:        handle,
				OpenFlags: flags,
			},
		})
	}
}


func doReadDir(state *MountState, req *request) {
	entries, code := state.fileSystem.ReadDir(req.inHeader, (*ReadIn)(req.inData))
	if entries != nil {
		req.flatData = entries.Bytes()
	}
	req.status = code
}


func doOpenDir(state *MountState, req *request) {
	flags, handle, status := state.fileSystem.OpenDir(req.inHeader, (*OpenIn)(req.inData))
	req.status = status
	if status.Ok() {
		req.outData = unsafe.Pointer(&OpenOut{
			Fh:        handle,
			OpenFlags: flags,
		})
	}
}

func doSetattr(state *MountState, req *request) {
	o, s := state.fileSystem.SetAttr(req.inHeader, (*SetAttrIn)(req.inData))
	req.outData = unsafe.Pointer(o)
	req.status = s
}

func doWrite(state *MountState, req *request) {
	n, status := state.fileSystem.Write((*WriteIn)(req.inData), req.arg)
	o := &WriteOut{
		Size: n,
	}
	req.outData = unsafe.Pointer(o)
	req.status = status
}


func doGetXAttr(state *MountState, req *request) {
	input := (*GetXAttrIn)(req.inData)
	var data []byte
	if req.inHeader.opcode == _OP_GETXATTR {
		data, req.status = state.fileSystem.GetXAttr(req.inHeader, req.filenames[0])
	} else {
		data, req.status = state.fileSystem.ListXAttr(req.inHeader)
	}

	if req.status != OK {
		return
	}

	size := uint32(len(data))
	if input.Size == 0 {
		out := &GetXAttrOut{
			Size: size,
		}
		req.outData = unsafe.Pointer(out)
	}

	if size > input.Size {
		req.status = ERANGE
	}

	req.flatData = data
}

func doGetAttr(state *MountState, req *request) {
	attrOut, s := state.fileSystem.GetAttr(req.inHeader, (*GetAttrIn)(req.inData))
	req.status = s
	req.outData = unsafe.Pointer(attrOut)
}

func doForget(state *MountState, req *request) {
	state.fileSystem.Forget(req.inHeader, (*ForgetIn)(req.inData))
}

func doReadlink(state *MountState, req *request) {
	req.flatData, req.status = state.fileSystem.Readlink(req.inHeader)
}

func doDestroy(state *MountState, req *request) {
	state.fileSystem.Destroy(req.inHeader, (*InitIn)(req.inData))
}

func doLookup(state *MountState, req *request) {
	lookupOut, s := state.fileSystem.Lookup(req.inHeader, req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(lookupOut)
}

func doMknod(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mknod(req.inHeader, (*MknodIn)(req.inData), req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doMkdir(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mkdir(req.inHeader, (*MkdirIn)(req.inData), req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doUnlink(state *MountState, req *request) {
	req.status = state.fileSystem.Unlink(req.inHeader, req.filenames[0])
}

func doRmdir(state *MountState, req *request) {
	req.status = state.fileSystem.Rmdir(req.inHeader, req.filenames[0])
}

func doLink(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Link(req.inHeader, (*LinkIn)(req.inData), req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doRead(state *MountState, req *request) {
	req.flatData, req.status = state.fileSystem.Read((*ReadIn)(req.inData), state.buffers)
}

func doFlush(state *MountState, req *request) {
	req.status = state.fileSystem.Flush((*FlushIn)(req.inData))
}

func doRelease(state *MountState, req *request) {
	state.fileSystem.Release(req.inHeader, (*ReleaseIn)(req.inData))
}

func doFsync(state *MountState, req *request) {
	req.status = state.fileSystem.Fsync((*FsyncIn)(req.inData))
}

func doReleaseDir(state *MountState, req *request) {
	state.fileSystem.ReleaseDir(req.inHeader, (*ReleaseIn)(req.inData))
}

func doFsyncDir(state *MountState, req *request) {
	req.status = state.fileSystem.FsyncDir(req.inHeader, (*FsyncIn)(req.inData))
}

func doSetXAttr(state *MountState, req *request) {
	splits := bytes.Split(req.arg, []byte{0}, 2)
	req.status = state.fileSystem.SetXAttr(req.inHeader, (*SetXAttrIn)(req.inData), string(splits[0]), splits[1])
}

func doRemoveXAttr(state *MountState, req *request) {
	req.status = state.fileSystem.RemoveXAttr(req.inHeader, req.filenames[0])
}

func doAccess(state *MountState, req *request) {
	req.status = state.fileSystem.Access(req.inHeader, (*AccessIn)(req.inData))
}

func doSymlink(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Symlink(req.inHeader, req.filenames[1], req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doRename(state *MountState, req *request) {
	req.status = state.fileSystem.Rename(req.inHeader, (*RenameIn)(req.inData), req.filenames[0], req.filenames[1])
}

func doIoctl(state *MountState, req *request) {
	out, data, stat := state.fileSystem.Ioctl(req.inHeader, (*IoctlIn)(req.inData))
	req.outData = unsafe.Pointer(out)
	req.flatData = data
	req.status = stat
}

////////////////////////////////////////////////////////////////

type operationFunc func(*MountState, *request)
type castPointerFunc func(unsafe.Pointer) interface{}

type operationHandler struct {
	Name       string
	Func       operationFunc
	InputSize  int
	OutputSize int
	DecodeIn   castPointerFunc
	DecodeOut  castPointerFunc
	FileNames  int
}

var operationHandlers []*operationHandler

func operationName(op opcode) string {
	h := getHandler(op)
	if h == nil {
		return "unknown"
	}
	return h.Name
}

func (op opcode) String() string {
	return operationName(op)
}

func getHandler(o opcode) *operationHandler {
	if o >= _OPCODE_COUNT {
		return nil
	}
	return operationHandlers[o]
}

func init() {
	operationHandlers = make([]*operationHandler, _OPCODE_COUNT)
	for i, _ := range operationHandlers {
		operationHandlers[i] = &operationHandler{Name: "UNKNOWN"}
	}

	for op, sz := range map[opcode]int{
		_OP_FORGET:     unsafe.Sizeof(ForgetIn{}),
		_OP_GETATTR:    unsafe.Sizeof(GetAttrIn{}),
		_OP_SETATTR:    unsafe.Sizeof(SetAttrIn{}),
		_OP_MKNOD:      unsafe.Sizeof(MknodIn{}),
		_OP_MKDIR:      unsafe.Sizeof(MkdirIn{}),
		_OP_RENAME:     unsafe.Sizeof(RenameIn{}),
		_OP_LINK:       unsafe.Sizeof(LinkIn{}),
		_OP_OPEN:       unsafe.Sizeof(OpenIn{}),
		_OP_READ:       unsafe.Sizeof(ReadIn{}),
		_OP_WRITE:      unsafe.Sizeof(WriteIn{}),
		_OP_RELEASE:    unsafe.Sizeof(ReleaseIn{}),
		_OP_FSYNC:      unsafe.Sizeof(FsyncIn{}),
		_OP_SETXATTR:   unsafe.Sizeof(SetXAttrIn{}),
		_OP_GETXATTR:   unsafe.Sizeof(GetXAttrIn{}),
		_OP_LISTXATTR:  unsafe.Sizeof(GetXAttrIn{}),
		_OP_FLUSH:      unsafe.Sizeof(FlushIn{}),
		_OP_INIT:       unsafe.Sizeof(InitIn{}),
		_OP_OPENDIR:    unsafe.Sizeof(OpenIn{}),
		_OP_READDIR:    unsafe.Sizeof(ReadIn{}),
		_OP_RELEASEDIR: unsafe.Sizeof(ReleaseIn{}),
		_OP_FSYNCDIR:   unsafe.Sizeof(FsyncIn{}),
		_OP_ACCESS:     unsafe.Sizeof(AccessIn{}),
		_OP_CREATE:     unsafe.Sizeof(CreateIn{}),
		_OP_INTERRUPT:  unsafe.Sizeof(InterruptIn{}),
		_OP_BMAP:       unsafe.Sizeof(BmapIn{}),
		_OP_IOCTL:      unsafe.Sizeof(IoctlIn{}),
		_OP_POLL:       unsafe.Sizeof(PollIn{}),
	} {
		operationHandlers[op].InputSize = sz
	}

	for op, sz := range map[opcode]int{
		_OP_LOOKUP:    unsafe.Sizeof(EntryOut{}),
		_OP_GETATTR:   unsafe.Sizeof(AttrOut{}),
		_OP_SETATTR:   unsafe.Sizeof(AttrOut{}),
		_OP_SYMLINK:   unsafe.Sizeof(EntryOut{}),
		_OP_MKNOD:     unsafe.Sizeof(EntryOut{}),
		_OP_MKDIR:     unsafe.Sizeof(EntryOut{}),
		_OP_LINK:      unsafe.Sizeof(EntryOut{}),
		_OP_OPEN:      unsafe.Sizeof(OpenOut{}),
		_OP_WRITE:     unsafe.Sizeof(WriteOut{}),
		_OP_STATFS:    unsafe.Sizeof(StatfsOut{}),
		_OP_GETXATTR:  unsafe.Sizeof(GetXAttrOut{}),
		_OP_LISTXATTR: unsafe.Sizeof(GetXAttrOut{}),
		_OP_INIT:      unsafe.Sizeof(InitOut{}),
		_OP_OPENDIR:   unsafe.Sizeof(OpenOut{}),
		_OP_CREATE:    unsafe.Sizeof(CreateOut{}),
		_OP_BMAP:      unsafe.Sizeof(BmapOut{}),
		_OP_IOCTL:     unsafe.Sizeof(IoctlOut{}),
		_OP_POLL:      unsafe.Sizeof(PollOut{}),
	} {
		operationHandlers[op].OutputSize = sz
	}

	for op, v := range map[opcode]string{
		_OP_LOOKUP:      "LOOKUP",
		_OP_FORGET:      "FORGET",
		_OP_GETATTR:     "GETATTR",
		_OP_SETATTR:     "SETATTR",
		_OP_READLINK:    "READLINK",
		_OP_SYMLINK:     "SYMLINK",
		_OP_MKNOD:       "MKNOD",
		_OP_MKDIR:       "MKDIR",
		_OP_UNLINK:      "UNLINK",
		_OP_RMDIR:       "RMDIR",
		_OP_RENAME:      "RENAME",
		_OP_LINK:        "LINK",
		_OP_OPEN:        "OPEN",
		_OP_READ:        "READ",
		_OP_WRITE:       "WRITE",
		_OP_STATFS:      "STATFS",
		_OP_RELEASE:     "RELEASE",
		_OP_FSYNC:       "FSYNC",
		_OP_SETXATTR:    "SETXATTR",
		_OP_GETXATTR:    "GETXATTR",
		_OP_LISTXATTR:   "LISTXATTR",
		_OP_REMOVEXATTR: "REMOVEXATTR",
		_OP_FLUSH:       "FLUSH",
		_OP_INIT:        "INIT",
		_OP_OPENDIR:     "OPENDIR",
		_OP_READDIR:     "READDIR",
		_OP_RELEASEDIR:  "RELEASEDIR",
		_OP_FSYNCDIR:    "FSYNCDIR",
		_OP_GETLK:       "GETLK",
		_OP_SETLK:       "SETLK",
		_OP_SETLKW:      "SETLKW",
		_OP_ACCESS:      "ACCESS",
		_OP_CREATE:      "CREATE",
		_OP_INTERRUPT:   "INTERRUPT",
		_OP_BMAP:        "BMAP",
		_OP_DESTROY:     "DESTROY",
		_OP_IOCTL:       "IOCTL",
		_OP_POLL:        "POLL"} {
		operationHandlers[op].Name = v
	}

	for op, v := range map[opcode]operationFunc{
		_OP_OPEN:        doOpen,
		_OP_READDIR:     doReadDir,
		_OP_WRITE:       doWrite,
		_OP_OPENDIR:     doOpenDir,
		_OP_CREATE:      doCreate,
		_OP_SETATTR:     doSetattr,
		_OP_GETXATTR:    doGetXAttr,
		_OP_LISTXATTR:   doGetXAttr,
		_OP_GETATTR:     doGetAttr,
		_OP_FORGET:      doForget,
		_OP_READLINK:    doReadlink,
		_OP_INIT:        doInit,
		_OP_DESTROY:     doDestroy,
		_OP_LOOKUP:      doLookup,
		_OP_MKNOD:       doMknod,
		_OP_MKDIR:       doMkdir,
		_OP_UNLINK:      doUnlink,
		_OP_RMDIR:       doRmdir,
		_OP_LINK:        doLink,
		_OP_READ:        doRead,
		_OP_FLUSH:       doFlush,
		_OP_RELEASE:     doRelease,
		_OP_FSYNC:       doFsync,
		_OP_RELEASEDIR:  doReleaseDir,
		_OP_FSYNCDIR:    doFsyncDir,
		_OP_SETXATTR:    doSetXAttr,
		_OP_REMOVEXATTR: doRemoveXAttr,
		_OP_ACCESS:      doAccess,
		_OP_SYMLINK:     doSymlink,
		_OP_RENAME:      doRename,
	} {
		operationHandlers[op].Func = v
	}

	// Outputs.
	for op, f := range map[opcode]castPointerFunc{
		_OP_LOOKUP:  func(ptr unsafe.Pointer) interface{} { return (*EntryOut)(ptr) },
		_OP_OPEN:    func(ptr unsafe.Pointer) interface{} { return (*OpenOut)(ptr) },
		_OP_GETATTR: func(ptr unsafe.Pointer) interface{} { return (*AttrOut)(ptr) },
		_OP_CREATE:  func(ptr unsafe.Pointer) interface{} { return (*CreateOut)(ptr) },
		_OP_SETATTR: func(ptr unsafe.Pointer) interface{} { return (*AttrOut)(ptr) },
		_OP_INIT:    func(ptr unsafe.Pointer) interface{} { return (*InitOut)(ptr) },
	} {
		operationHandlers[op].DecodeOut = f
	}

	// Inputs.
	for op, f := range map[opcode]castPointerFunc{
		_OP_GETATTR: func(ptr unsafe.Pointer) interface{} { return (*GetAttrIn)(ptr) },
		_OP_SETATTR: func(ptr unsafe.Pointer) interface{} { return (*SetAttrIn)(ptr) },
		_OP_INIT:    func(ptr unsafe.Pointer) interface{} { return (*InitIn)(ptr) },
		_OP_IOCTL:   func(ptr unsafe.Pointer) interface{} { return (*IoctlIn)(ptr) },
		_OP_OPEN:    func(ptr unsafe.Pointer) interface{} { return (*OpenIn)(ptr) },
		_OP_CREATE:  func(ptr unsafe.Pointer) interface{} { return (*CreateIn)(ptr) },
		_OP_READ:    func(ptr unsafe.Pointer) interface{} { return (*ReadIn)(ptr) },
	} {
		operationHandlers[op].DecodeIn = f
	}

	// File name args.
	for op, count := range map[opcode]int{
		_OP_CREATE:      1,
		_OP_GETXATTR:    1,
		_OP_LINK:        1,
		_OP_LOOKUP:      1,
		_OP_MKDIR:       1,
		_OP_MKNOD:       1,
		_OP_REMOVEXATTR: 1,
		_OP_RENAME:      2,
		_OP_RMDIR:       1,
		_OP_SYMLINK:     2,
		_OP_UNLINK:      1,
	} {
		operationHandlers[op].FileNames = count
	}
}
