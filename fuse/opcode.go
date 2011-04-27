package fuse

import (
	"bytes"
	"fmt"
	"log"
	"unsafe"
)

var _ = log.Printf

func replyString(opcode Opcode, ptr unsafe.Pointer) string {
	h := getHandler(opcode)
	var val interface{}
	if h.DecodeOut != nil {
		val = h.DecodeOut(ptr)
	}
	if val != nil {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

////////////////////////////////////////////////////////////////

func doInit(state *MountState, req *request) {
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

	out := &InitOut{
	Major: FUSE_KERNEL_VERSION,
	Minor: FUSE_KERNEL_MINOR_VERSION,
	MaxReadAhead: input.MaxReadAhead,
	Flags: CAP_ASYNC_READ | CAP_POSIX_LOCKS | CAP_BIG_WRITES,
	MaxWrite: maxRead,
	CongestionThreshold: _BACKGROUND_TASKS * 3 / 4,
	MaxBackground: _BACKGROUND_TASKS,
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
	flags, handle, entry, status := state.fileSystem.Create(req.inHeader, (*CreateIn)(req.inData), req.filename())
	req.status = status
	if status == OK {
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
	if status == OK {
		req.outData = unsafe.Pointer(&OpenOut{
			Fh:        handle,
			OpenFlags: flags,
		})
	}
}

func doSetattr(state *MountState, req *request) {
	// TODO - if Fh != 0, we should do a FSetAttr instead.
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
	if req.inHeader.Opcode == FUSE_GETXATTR {
		data, req.status = state.fileSystem.GetXAttr(req.inHeader, req.filename())
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
	// TODO - if req.inData.Fh is set, do file.GetAttr
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
	lookupOut, s := state.fileSystem.Lookup(req.inHeader, req.filename())
	req.status = s
	req.outData = unsafe.Pointer(lookupOut)
}

func doMknod(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mknod(req.inHeader, (*MknodIn)(req.inData), req.filename())
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doMkdir(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mkdir(req.inHeader, (*MkdirIn)(req.inData), req.filename())
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doUnlink(state *MountState, req *request) {
	req.status = state.fileSystem.Unlink(req.inHeader, req.filename())
}

func doRmdir(state *MountState, req *request) {
	req.status = state.fileSystem.Rmdir(req.inHeader, req.filename())
}

func doLink(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Link(req.inHeader, (*LinkIn)(req.inData), req.filename())
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
	req.status = state.fileSystem.RemoveXAttr(req.inHeader, req.filename())
}

func doAccess(state *MountState, req *request) {
	req.status = state.fileSystem.Access(req.inHeader, (*AccessIn)(req.inData))
}

func doSymlink(state *MountState, req *request) {
	filenames := req.filenames(3)
	if len(filenames) >= 2 {
		entryOut, s := state.fileSystem.Symlink(req.inHeader, filenames[1], filenames[0])
		req.status = s
		req.outData = unsafe.Pointer(entryOut)
	} else {
		log.Println("Symlink: missing arguments", filenames)
		req.status = EIO
	}
}

func doRename(state *MountState, req *request) {
	filenames := req.filenames(3)
	if len(filenames) >= 2 {
		req.status = state.fileSystem.Rename(req.inHeader, (*RenameIn)(req.inData), filenames[0], filenames[1])
	} else {
		log.Println("Rename: missing arguments", filenames)
		req.status = EIO
	}
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
}

var operationHandlers []*operationHandler

func operationName(opcode Opcode) string {
	h := getHandler(opcode)
	if h == nil {
		return "unknown"
	}
	return h.Name
}

func getHandler(o Opcode) *operationHandler {
	if o >= OPCODE_COUNT {
		return nil
	}
	return operationHandlers[o]
}

func init() {
	operationHandlers = make([]*operationHandler, OPCODE_COUNT)
	for i, _ := range operationHandlers {
		operationHandlers[i] = &operationHandler{Name: "UNKNOWN"}
	}

	for op, sz := range map[int]int{
		FUSE_FORGET:     unsafe.Sizeof(ForgetIn{}),
		FUSE_GETATTR:    unsafe.Sizeof(GetAttrIn{}),
		FUSE_SETATTR:    unsafe.Sizeof(SetAttrIn{}),
		FUSE_MKNOD:      unsafe.Sizeof(MknodIn{}),
		FUSE_MKDIR:      unsafe.Sizeof(MkdirIn{}),
		FUSE_RENAME:     unsafe.Sizeof(RenameIn{}),
		FUSE_LINK:       unsafe.Sizeof(LinkIn{}),
		FUSE_OPEN:       unsafe.Sizeof(OpenIn{}),
		FUSE_READ:       unsafe.Sizeof(ReadIn{}),
		FUSE_WRITE:      unsafe.Sizeof(WriteIn{}),
		FUSE_RELEASE:    unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNC:      unsafe.Sizeof(FsyncIn{}),
		FUSE_SETXATTR:   unsafe.Sizeof(SetXAttrIn{}),
		FUSE_GETXATTR:   unsafe.Sizeof(GetXAttrIn{}),
		FUSE_LISTXATTR:  unsafe.Sizeof(GetXAttrIn{}),
		FUSE_FLUSH:      unsafe.Sizeof(FlushIn{}),
		FUSE_INIT:       unsafe.Sizeof(InitIn{}),
		FUSE_OPENDIR:    unsafe.Sizeof(OpenIn{}),
		FUSE_READDIR:    unsafe.Sizeof(ReadIn{}),
		FUSE_RELEASEDIR: unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNCDIR:   unsafe.Sizeof(FsyncIn{}),
		FUSE_ACCESS:     unsafe.Sizeof(AccessIn{}),
		FUSE_CREATE:     unsafe.Sizeof(CreateIn{}),
		FUSE_INTERRUPT:  unsafe.Sizeof(InterruptIn{}),
		FUSE_BMAP:       unsafe.Sizeof(BmapIn{}),
		FUSE_IOCTL:      unsafe.Sizeof(IoctlIn{}),
		FUSE_POLL:       unsafe.Sizeof(PollIn{}),
	} {
		operationHandlers[op].InputSize = sz
	}

	for op, sz := range map[int]int{
		FUSE_LOOKUP:    unsafe.Sizeof(EntryOut{}),
		FUSE_GETATTR:   unsafe.Sizeof(AttrOut{}),
		FUSE_SETATTR:   unsafe.Sizeof(AttrOut{}),
		FUSE_SYMLINK:   unsafe.Sizeof(EntryOut{}),
		FUSE_MKNOD:     unsafe.Sizeof(EntryOut{}),
		FUSE_MKDIR:     unsafe.Sizeof(EntryOut{}),
		FUSE_LINK:      unsafe.Sizeof(EntryOut{}),
		FUSE_OPEN:      unsafe.Sizeof(OpenOut{}),
		FUSE_WRITE:     unsafe.Sizeof(WriteOut{}),
		FUSE_STATFS:    unsafe.Sizeof(StatfsOut{}),
		FUSE_GETXATTR:  unsafe.Sizeof(GetXAttrOut{}),
		FUSE_LISTXATTR: unsafe.Sizeof(GetXAttrOut{}),
		FUSE_INIT:      unsafe.Sizeof(InitOut{}),
		FUSE_OPENDIR:   unsafe.Sizeof(OpenOut{}),
		FUSE_CREATE:    unsafe.Sizeof(CreateOut{}),
		FUSE_BMAP:      unsafe.Sizeof(BmapOut{}),
		FUSE_IOCTL:     unsafe.Sizeof(IoctlOut{}),
		FUSE_POLL:      unsafe.Sizeof(PollOut{}),
	} {
		operationHandlers[op].OutputSize = sz
	}

	for op, v := range map[int]string{
		FUSE_LOOKUP:      "FUSE_LOOKUP",
		FUSE_FORGET:      "FUSE_FORGET",
		FUSE_GETATTR:     "FUSE_GETATTR",
		FUSE_SETATTR:     "FUSE_SETATTR",
		FUSE_READLINK:    "FUSE_READLINK",
		FUSE_SYMLINK:     "FUSE_SYMLINK",
		FUSE_MKNOD:       "FUSE_MKNOD",
		FUSE_MKDIR:       "FUSE_MKDIR",
		FUSE_UNLINK:      "FUSE_UNLINK",
		FUSE_RMDIR:       "FUSE_RMDIR",
		FUSE_RENAME:      "FUSE_RENAME",
		FUSE_LINK:        "FUSE_LINK",
		FUSE_OPEN:        "FUSE_OPEN",
		FUSE_READ:        "FUSE_READ",
		FUSE_WRITE:       "FUSE_WRITE",
		FUSE_STATFS:      "FUSE_STATFS",
		FUSE_RELEASE:     "FUSE_RELEASE",
		FUSE_FSYNC:       "FUSE_FSYNC",
		FUSE_SETXATTR:    "FUSE_SETXATTR",
		FUSE_GETXATTR:    "FUSE_GETXATTR",
		FUSE_LISTXATTR:   "FUSE_LISTXATTR",
		FUSE_REMOVEXATTR: "FUSE_REMOVEXATTR",
		FUSE_FLUSH:       "FUSE_FLUSH",
		FUSE_INIT:        "FUSE_INIT",
		FUSE_OPENDIR:     "FUSE_OPENDIR",
		FUSE_READDIR:     "FUSE_READDIR",
		FUSE_RELEASEDIR:  "FUSE_RELEASEDIR",
		FUSE_FSYNCDIR:    "FUSE_FSYNCDIR",
		FUSE_GETLK:       "FUSE_GETLK",
		FUSE_SETLK:       "FUSE_SETLK",
		FUSE_SETLKW:      "FUSE_SETLKW",
		FUSE_ACCESS:      "FUSE_ACCESS",
		FUSE_CREATE:      "FUSE_CREATE",
		FUSE_INTERRUPT:   "FUSE_INTERRUPT",
		FUSE_BMAP:        "FUSE_BMAP",
		FUSE_DESTROY:     "FUSE_DESTROY",
		FUSE_IOCTL:       "FUSE_IOCTL",
		FUSE_POLL:        "FUSE_POLL"} {
		operationHandlers[op].Name = v
	}

	for op, v := range map[Opcode]operationFunc{
		FUSE_OPEN:        doOpen,
		FUSE_READDIR:     doReadDir,
		FUSE_WRITE:       doWrite,
		FUSE_OPENDIR:     doOpenDir,
		FUSE_CREATE:      doCreate,
		FUSE_SETATTR:     doSetattr,
		FUSE_GETXATTR:    doGetXAttr,
		FUSE_LISTXATTR:   doGetXAttr,
		FUSE_GETATTR:     doGetAttr,
		FUSE_FORGET:      doForget,
		FUSE_READLINK:    doReadlink,
		FUSE_INIT:        doInit,
		FUSE_DESTROY:     doDestroy,
		FUSE_LOOKUP:      doLookup,
		FUSE_MKNOD:       doMknod,
		FUSE_MKDIR:       doMkdir,
		FUSE_UNLINK:      doUnlink,
		FUSE_RMDIR:       doRmdir,
		FUSE_LINK:        doLink,
		FUSE_READ:        doRead,
		FUSE_FLUSH:       doFlush,
		FUSE_RELEASE:     doRelease,
		FUSE_FSYNC:       doFsync,
		FUSE_RELEASEDIR:  doReleaseDir,
		FUSE_FSYNCDIR:    doFsyncDir,
		FUSE_SETXATTR:    doSetXAttr,
		FUSE_REMOVEXATTR: doRemoveXAttr,
		FUSE_ACCESS:      doAccess,
		FUSE_SYMLINK:     doSymlink,
		FUSE_RENAME:      doRename,
	} {
		operationHandlers[op].Func = v
	}

	for op, f := range map[Opcode]castPointerFunc{
		FUSE_LOOKUP: func(ptr unsafe.Pointer) interface{} { return (*EntryOut)(ptr) },
		FUSE_OPEN: func(ptr unsafe.Pointer) interface{} { return (*EntryOut)(ptr) },
		FUSE_GETATTR: func(ptr unsafe.Pointer) interface{} { return (*AttrOut)(ptr) },
	} {
		operationHandlers[op].DecodeOut = f
	}

}
