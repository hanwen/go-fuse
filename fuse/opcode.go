package fuse

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Printf
var _ = fmt.Printf

const (
	_OP_LOOKUP       = int32(1)
	_OP_FORGET       = int32(2)
	_OP_GETATTR      = int32(3)
	_OP_SETATTR      = int32(4)
	_OP_READLINK     = int32(5)
	_OP_SYMLINK      = int32(6)
	_OP_MKNOD        = int32(8)
	_OP_MKDIR        = int32(9)
	_OP_UNLINK       = int32(10)
	_OP_RMDIR        = int32(11)
	_OP_RENAME       = int32(12)
	_OP_LINK         = int32(13)
	_OP_OPEN         = int32(14)
	_OP_READ         = int32(15)
	_OP_WRITE        = int32(16)
	_OP_STATFS       = int32(17)
	_OP_RELEASE      = int32(18)
	_OP_FSYNC        = int32(20)
	_OP_SETXATTR     = int32(21)
	_OP_GETXATTR     = int32(22)
	_OP_LISTXATTR    = int32(23)
	_OP_REMOVEXATTR  = int32(24)
	_OP_FLUSH        = int32(25)
	_OP_INIT         = int32(26)
	_OP_OPENDIR      = int32(27)
	_OP_READDIR      = int32(28)
	_OP_RELEASEDIR   = int32(29)
	_OP_FSYNCDIR     = int32(30)
	_OP_GETLK        = int32(31)
	_OP_SETLK        = int32(32)
	_OP_SETLKW       = int32(33)
	_OP_ACCESS       = int32(34)
	_OP_CREATE       = int32(35)
	_OP_INTERRUPT    = int32(36)
	_OP_BMAP         = int32(37)
	_OP_DESTROY      = int32(38)
	_OP_IOCTL        = int32(39)
	_OP_POLL         = int32(40)
	_OP_NOTIFY_REPLY = int32(41)
	_OP_BATCH_FORGET = int32(42)

	// Ugh - what will happen if FUSE introduces a new opcode here?
	_OP_NOTIFY_ENTRY = int32(51)
	_OP_NOTIFY_INODE = int32(52)

	_OPCODE_COUNT = int32(53)
)

////////////////////////////////////////////////////////////////

func doInit(state *MountState, req *request) {
	const (
		FUSE_KERNEL_VERSION       = 7
		MINIMUM_MINOR_VERSION = 13
		OUR_MINOR_VERSION = 16
	)

	input := (*raw.InitIn)(req.inData)
	if input.Major != FUSE_KERNEL_VERSION {
		log.Printf("Major versions does not match. Given %d, want %d\n", input.Major, FUSE_KERNEL_VERSION)
		req.status = EIO
		return
	}
	if input.Minor < MINIMUM_MINOR_VERSION {
		log.Printf("Minor version is less than we support. Given %d, want at least %d\n", input.Minor, MINIMUM_MINOR_VERSION)
		req.status = EIO
		return
	}

	state.kernelSettings = *input
	state.kernelSettings.Flags = input.Flags & (raw.CAP_ASYNC_READ | raw.CAP_BIG_WRITES | raw.CAP_FILE_OPS)
	out := &raw.InitOut{
		Major:               FUSE_KERNEL_VERSION,
		Minor:               OUR_MINOR_VERSION,
		MaxReadAhead:        input.MaxReadAhead,
		Flags:               state.kernelSettings.Flags,
		MaxWrite:            uint32(state.opts.MaxWrite),
		CongestionThreshold: uint16(state.opts.MaxBackground * 3 / 4),
		MaxBackground:       uint16(state.opts.MaxBackground),
	}
	if out.Minor > input.Minor {
		out.Minor = input.Minor
	}
	
	req.outData = unsafe.Pointer(out)
	req.status = OK
}

func doOpen(state *MountState, req *request) {
	flags, handle, status := state.fileSystem.Open(req.inHeader, (*raw.OpenIn)(req.inData))
	req.status = status
	if status != OK {
		return
	}

	out := &raw.OpenOut{
		Fh:        handle,
		OpenFlags: flags,
	}

	req.outData = unsafe.Pointer(out)
}

func doCreate(state *MountState, req *request) {
	flags, handle, entry, status := state.fileSystem.Create(req.inHeader, (*raw.CreateIn)(req.inData), req.filenames[0])
	req.status = status
	if status.Ok() {
		req.outData = unsafe.Pointer(&raw.CreateOut{
			raw.EntryOut: *entry,
			raw.OpenOut: raw.OpenOut{
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
	flags, handle, status := state.fileSystem.OpenDir(req.inHeader, (*raw.OpenIn)(req.inData))
	req.status = status
	if status.Ok() {
		req.outData = unsafe.Pointer(&raw.OpenOut{
			Fh:        handle,
			OpenFlags: flags,
		})
	}
}

func doSetattr(state *MountState, req *request) {
	o, s := state.fileSystem.SetAttr(req.inHeader, (*raw.SetAttrIn)(req.inData))
	req.outData = unsafe.Pointer(o)
	req.status = s
}

func doWrite(state *MountState, req *request) {
	n, status := state.fileSystem.Write(req.inHeader, (*WriteIn)(req.inData), req.arg)
	o := &raw.WriteOut{
		Size: n,
	}
	req.outData = unsafe.Pointer(o)
	req.status = status
}

const _SECURITY_CAPABILITY = "security.capability"
const _SECURITY_ACL = "system.posix_acl_access"
const _SECURITY_ACL_DEFAULT = "system.posix_acl_default"

func doGetXAttr(state *MountState, req *request) {
	if state.opts.IgnoreSecurityLabels && req.inHeader.Opcode == _OP_GETXATTR {
		fn := req.filenames[0]
		if fn == _SECURITY_CAPABILITY || fn == _SECURITY_ACL_DEFAULT ||
			fn == _SECURITY_ACL {
			req.status = ENODATA
			return
		}
	}

	input := (*raw.GetXAttrIn)(req.inData)
	var data []byte
	switch {
	case req.inHeader.Opcode == _OP_GETXATTR && input.Size == 0:
		sz, code := state.fileSystem.GetXAttrSize(req.inHeader, req.filenames[0])
		if code.Ok() {
			out := &raw.GetXAttrOut{
				Size: uint32(sz),
			}
			req.outData = unsafe.Pointer(out)
			req.status = ERANGE
			return
		}
		req.status = code
	case req.inHeader.Opcode == _OP_GETXATTR:
		data, req.status = state.fileSystem.GetXAttrData(req.inHeader, req.filenames[0])
	default:
		data, req.status = state.fileSystem.ListXAttr(req.inHeader)
	}

	if len(data) > int(input.Size) {
		req.status = ERANGE
	}

	if !req.status.Ok() {
		return
	}

	req.flatData = data
}

func doGetAttr(state *MountState, req *request) {
	attrOut, s := state.fileSystem.GetAttr(req.inHeader, (*raw.GetAttrIn)(req.inData))
	req.status = s
	req.outData = unsafe.Pointer(attrOut)
}

func doForget(state *MountState, req *request) {
	state.fileSystem.Forget(req.inHeader.NodeId, (*raw.ForgetIn)(req.inData).Nlookup)
}

func doBatchForget(state *MountState, req *request) {
	in := (*raw.BatchForgetIn)(req.inData)
	wantBytes := uintptr(in.Count)*unsafe.Sizeof(raw.BatchForgetIn{})
	if uintptr(len(req.arg)) < wantBytes {
		// We have no return value to complain, so log an error.
		log.Printf("Too few bytes for batch forget. Got %d bytes, want %d (%d entries)",
			len(req.arg), wantBytes, in.Count)
	}

	h := &reflect.SliceHeader{uintptr(unsafe.Pointer(&req.arg[0])), int(in.Count), int(in.Count)}

	forgets := *(*[]raw.ForgetOne)(unsafe.Pointer(h))
	for _, f := range forgets {
		state.fileSystem.Forget(f.NodeId, f.Nlookup)
	}
}

func doReadlink(state *MountState, req *request) {
	req.flatData, req.status = state.fileSystem.Readlink(req.inHeader)
}

func doLookup(state *MountState, req *request) {
	lookupOut, s := state.fileSystem.Lookup(req.inHeader, req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(lookupOut)
}

func doMknod(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mknod(req.inHeader, (*raw.MknodIn)(req.inData), req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doMkdir(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Mkdir(req.inHeader, (*raw.MkdirIn)(req.inData), req.filenames[0])
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
	entryOut, s := state.fileSystem.Link(req.inHeader, (*raw.LinkIn)(req.inData), req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doRead(state *MountState, req *request) {
	req.flatData, req.status = state.fileSystem.Read(req.inHeader, (*ReadIn)(req.inData), state.buffers)
}

func doFlush(state *MountState, req *request) {
	req.status = state.fileSystem.Flush(req.inHeader, (*raw.FlushIn)(req.inData))
}

func doRelease(state *MountState, req *request) {
	state.fileSystem.Release(req.inHeader, (*raw.ReleaseIn)(req.inData))
}

func doFsync(state *MountState, req *request) {
	req.status = state.fileSystem.Fsync(req.inHeader, (*raw.FsyncIn)(req.inData))
}

func doReleaseDir(state *MountState, req *request) {
	state.fileSystem.ReleaseDir(req.inHeader, (*raw.ReleaseIn)(req.inData))
}

func doFsyncDir(state *MountState, req *request) {
	req.status = state.fileSystem.FsyncDir(req.inHeader, (*raw.FsyncIn)(req.inData))
}

func doSetXAttr(state *MountState, req *request) {
	splits := bytes.SplitN(req.arg, []byte{0}, 2)
	req.status = state.fileSystem.SetXAttr(req.inHeader, (*raw.SetXAttrIn)(req.inData), string(splits[0]), splits[1])
}

func doRemoveXAttr(state *MountState, req *request) {
	req.status = state.fileSystem.RemoveXAttr(req.inHeader, req.filenames[0])
}

func doAccess(state *MountState, req *request) {
	req.status = state.fileSystem.Access(req.inHeader, (*raw.AccessIn)(req.inData))
}

func doSymlink(state *MountState, req *request) {
	entryOut, s := state.fileSystem.Symlink(req.inHeader, req.filenames[1], req.filenames[0])
	req.status = s
	req.outData = unsafe.Pointer(entryOut)
}

func doRename(state *MountState, req *request) {
	req.status = state.fileSystem.Rename(req.inHeader, (*raw.RenameIn)(req.inData), req.filenames[0], req.filenames[1])
}

func doStatFs(state *MountState, req *request) {
	stat := state.fileSystem.StatFs(req.inHeader)
	if stat != nil {
		req.outData = unsafe.Pointer(stat)
		req.status = OK
	} else {
		req.status = ENOSYS
	}
}

func doIoctl(state *MountState, req *request) {
	out, data, stat := state.fileSystem.Ioctl(req.inHeader, (*raw.IoctlIn)(req.inData))
	req.outData = unsafe.Pointer(out)
	req.flatData = data
	req.status = stat
}

////////////////////////////////////////////////////////////////

type operationFunc func(*MountState, *request)
type castPointerFunc func(unsafe.Pointer) interface{}

type operationHandler struct {
	Name        string
	Func        operationFunc
	InputSize   uintptr
	OutputSize  uintptr
	DecodeIn    castPointerFunc
	DecodeOut   castPointerFunc
	FileNames   int
	FileNameOut bool
}

var operationHandlers []*operationHandler

func operationName(op int32) string {
	h := getHandler(op)
	if h == nil {
		return "unknown"
	}
	return h.Name
}

func getHandler(o int32) *operationHandler {
	if o >= _OPCODE_COUNT {
		return nil
	}
	return operationHandlers[o]
}

func init() {
	operationHandlers = make([]*operationHandler, _OPCODE_COUNT)
	for i := range operationHandlers {
		operationHandlers[i] = &operationHandler{Name: "UNKNOWN"}
	}

	fileOps := []int32{_OP_READLINK, _OP_NOTIFY_ENTRY}
	for _, op := range fileOps {
		operationHandlers[op].FileNameOut = true
	}

	for op, sz := range map[int32]uintptr{
		_OP_FORGET:       unsafe.Sizeof(raw.ForgetIn{}),
		_OP_BATCH_FORGET: unsafe.Sizeof(raw.BatchForgetIn{}),
		_OP_GETATTR:      unsafe.Sizeof(raw.GetAttrIn{}),
		_OP_SETATTR:      unsafe.Sizeof(raw.SetAttrIn{}),
		_OP_MKNOD:        unsafe.Sizeof(raw.MknodIn{}),
		_OP_MKDIR:        unsafe.Sizeof(raw.MkdirIn{}),
		_OP_RENAME:       unsafe.Sizeof(raw.RenameIn{}),
		_OP_LINK:         unsafe.Sizeof(raw.LinkIn{}),
		_OP_OPEN:         unsafe.Sizeof(raw.OpenIn{}),
		_OP_READ:         unsafe.Sizeof(ReadIn{}),
		_OP_WRITE:        unsafe.Sizeof(WriteIn{}),
		_OP_RELEASE:      unsafe.Sizeof(raw.ReleaseIn{}),
		_OP_FSYNC:        unsafe.Sizeof(raw.FsyncIn{}),
		_OP_SETXATTR:     unsafe.Sizeof(raw.SetXAttrIn{}),
		_OP_GETXATTR:     unsafe.Sizeof(raw.GetXAttrIn{}),
		_OP_LISTXATTR:    unsafe.Sizeof(raw.GetXAttrIn{}),
		_OP_FLUSH:        unsafe.Sizeof(raw.FlushIn{}),
		_OP_INIT:         unsafe.Sizeof(raw.InitIn{}),
		_OP_OPENDIR:      unsafe.Sizeof(raw.OpenIn{}),
		_OP_READDIR:      unsafe.Sizeof(ReadIn{}),
		_OP_RELEASEDIR:   unsafe.Sizeof(raw.ReleaseIn{}),
		_OP_FSYNCDIR:     unsafe.Sizeof(raw.FsyncIn{}),
		_OP_ACCESS:       unsafe.Sizeof(raw.AccessIn{}),
		_OP_CREATE:       unsafe.Sizeof(raw.CreateIn{}),
		_OP_INTERRUPT:    unsafe.Sizeof(raw.InterruptIn{}),
		_OP_BMAP:         unsafe.Sizeof(raw.BmapIn{}),
		_OP_IOCTL:        unsafe.Sizeof(raw.IoctlIn{}),
		_OP_POLL:         unsafe.Sizeof(raw.PollIn{}),
	} {
		operationHandlers[op].InputSize = sz
	}

	for op, sz := range map[int32]uintptr{
		_OP_LOOKUP:       unsafe.Sizeof(raw.EntryOut{}),
		_OP_GETATTR:      unsafe.Sizeof(raw.AttrOut{}),
		_OP_SETATTR:      unsafe.Sizeof(raw.AttrOut{}),
		_OP_SYMLINK:      unsafe.Sizeof(raw.EntryOut{}),
		_OP_MKNOD:        unsafe.Sizeof(raw.EntryOut{}),
		_OP_MKDIR:        unsafe.Sizeof(raw.EntryOut{}),
		_OP_LINK:         unsafe.Sizeof(raw.EntryOut{}),
		_OP_OPEN:         unsafe.Sizeof(raw.OpenOut{}),
		_OP_WRITE:        unsafe.Sizeof(raw.WriteOut{}),
		_OP_STATFS:       unsafe.Sizeof(StatfsOut{}),
		_OP_GETXATTR:     unsafe.Sizeof(raw.GetXAttrOut{}),
		_OP_LISTXATTR:    unsafe.Sizeof(raw.GetXAttrOut{}),
		_OP_INIT:         unsafe.Sizeof(raw.InitOut{}),
		_OP_OPENDIR:      unsafe.Sizeof(raw.OpenOut{}),
		_OP_CREATE:       unsafe.Sizeof(raw.CreateOut{}),
		_OP_BMAP:         unsafe.Sizeof(raw.BmapOut{}),
		_OP_IOCTL:        unsafe.Sizeof(raw.IoctlOut{}),
		_OP_POLL:         unsafe.Sizeof(raw.PollOut{}),
		_OP_NOTIFY_ENTRY: unsafe.Sizeof(raw.NotifyInvalEntryOut{}),
		_OP_NOTIFY_INODE: unsafe.Sizeof(raw.NotifyInvalInodeOut{}),
	} {
		operationHandlers[op].OutputSize = sz
	}

	for op, v := range map[int32]string{
		_OP_LOOKUP:       "LOOKUP",
		_OP_FORGET:       "FORGET",
		_OP_BATCH_FORGET: "BATCH_FORGET",
		_OP_GETATTR:      "GETATTR",
		_OP_SETATTR:      "SETATTR",
		_OP_READLINK:     "READLINK",
		_OP_SYMLINK:      "SYMLINK",
		_OP_MKNOD:        "MKNOD",
		_OP_MKDIR:        "MKDIR",
		_OP_UNLINK:       "UNLINK",
		_OP_RMDIR:        "RMDIR",
		_OP_RENAME:       "RENAME",
		_OP_LINK:         "LINK",
		_OP_OPEN:         "OPEN",
		_OP_READ:         "READ",
		_OP_WRITE:        "WRITE",
		_OP_STATFS:       "STATFS",
		_OP_RELEASE:      "RELEASE",
		_OP_FSYNC:        "FSYNC",
		_OP_SETXATTR:     "SETXATTR",
		_OP_GETXATTR:     "GETXATTR",
		_OP_LISTXATTR:    "LISTXATTR",
		_OP_REMOVEXATTR:  "REMOVEXATTR",
		_OP_FLUSH:        "FLUSH",
		_OP_INIT:         "INIT",
		_OP_OPENDIR:      "OPENDIR",
		_OP_READDIR:      "READDIR",
		_OP_RELEASEDIR:   "RELEASEDIR",
		_OP_FSYNCDIR:     "FSYNCDIR",
		_OP_GETLK:        "GETLK",
		_OP_SETLK:        "SETLK",
		_OP_SETLKW:       "SETLKW",
		_OP_ACCESS:       "ACCESS",
		_OP_CREATE:       "CREATE",
		_OP_INTERRUPT:    "INTERRUPT",
		_OP_BMAP:         "BMAP",
		_OP_DESTROY:      "DESTROY",
		_OP_IOCTL:        "IOCTL",
		_OP_POLL:         "POLL",
		_OP_NOTIFY_ENTRY: "NOTIFY_ENTRY",
		_OP_NOTIFY_INODE: "NOTIFY_INODE",
	} {
		operationHandlers[op].Name = v
	}

	for op, v := range map[int32]operationFunc{
		_OP_OPEN:         doOpen,
		_OP_READDIR:      doReadDir,
		_OP_WRITE:        doWrite,
		_OP_OPENDIR:      doOpenDir,
		_OP_CREATE:       doCreate,
		_OP_SETATTR:      doSetattr,
		_OP_GETXATTR:     doGetXAttr,
		_OP_LISTXATTR:    doGetXAttr,
		_OP_GETATTR:      doGetAttr,
		_OP_FORGET:       doForget,
		_OP_BATCH_FORGET: doBatchForget,
		_OP_READLINK:     doReadlink,
		_OP_INIT:         doInit,
		_OP_LOOKUP:       doLookup,
		_OP_MKNOD:        doMknod,
		_OP_MKDIR:        doMkdir,
		_OP_UNLINK:       doUnlink,
		_OP_RMDIR:        doRmdir,
		_OP_LINK:         doLink,
		_OP_READ:         doRead,
		_OP_FLUSH:        doFlush,
		_OP_RELEASE:      doRelease,
		_OP_FSYNC:        doFsync,
		_OP_RELEASEDIR:   doReleaseDir,
		_OP_FSYNCDIR:     doFsyncDir,
		_OP_SETXATTR:     doSetXAttr,
		_OP_REMOVEXATTR:  doRemoveXAttr,
		_OP_ACCESS:       doAccess,
		_OP_SYMLINK:      doSymlink,
		_OP_RENAME:       doRename,
		_OP_STATFS:       doStatFs,
		_OP_IOCTL:        doIoctl,
	} {
		operationHandlers[op].Func = v
	}

	// Outputs.
	for op, f := range map[int32]castPointerFunc{
		_OP_LOOKUP:       func(ptr unsafe.Pointer) interface{} { return (*raw.EntryOut)(ptr) },
		_OP_OPEN:         func(ptr unsafe.Pointer) interface{} { return (*raw.OpenOut)(ptr) },
		_OP_OPENDIR:      func(ptr unsafe.Pointer) interface{} { return (*raw.OpenOut)(ptr) },
		_OP_GETATTR:      func(ptr unsafe.Pointer) interface{} { return (*raw.AttrOut)(ptr) },
		_OP_CREATE:       func(ptr unsafe.Pointer) interface{} { return (*raw.CreateOut)(ptr) },
		_OP_LINK:         func(ptr unsafe.Pointer) interface{} { return (*raw.EntryOut)(ptr) },
		_OP_SETATTR:      func(ptr unsafe.Pointer) interface{} { return (*raw.AttrOut)(ptr) },
		_OP_INIT:         func(ptr unsafe.Pointer) interface{} { return (*raw.InitOut)(ptr) },
		_OP_MKDIR:        func(ptr unsafe.Pointer) interface{} { return (*raw.EntryOut)(ptr) },
		_OP_NOTIFY_ENTRY: func(ptr unsafe.Pointer) interface{} { return (*raw.NotifyInvalEntryOut)(ptr) },
		_OP_NOTIFY_INODE: func(ptr unsafe.Pointer) interface{} { return (*raw.NotifyInvalInodeOut)(ptr) },
		_OP_STATFS:       func(ptr unsafe.Pointer) interface{} { return (*StatfsOut)(ptr) },
	} {
		operationHandlers[op].DecodeOut = f
	}

	// Inputs.
	for op, f := range map[int32]castPointerFunc{
		_OP_FLUSH:        func(ptr unsafe.Pointer) interface{} { return (*raw.FlushIn)(ptr) },
		_OP_GETATTR:      func(ptr unsafe.Pointer) interface{} { return (*raw.GetAttrIn)(ptr) },
		_OP_SETATTR:      func(ptr unsafe.Pointer) interface{} { return (*raw.SetAttrIn)(ptr) },
		_OP_INIT:         func(ptr unsafe.Pointer) interface{} { return (*raw.InitIn)(ptr) },
		_OP_IOCTL:        func(ptr unsafe.Pointer) interface{} { return (*raw.IoctlIn)(ptr) },
		_OP_OPEN:         func(ptr unsafe.Pointer) interface{} { return (*raw.OpenIn)(ptr) },
		_OP_MKNOD:        func(ptr unsafe.Pointer) interface{} { return (*raw.MknodIn)(ptr) },
		_OP_CREATE:       func(ptr unsafe.Pointer) interface{} { return (*raw.CreateIn)(ptr) },
		_OP_READ:         func(ptr unsafe.Pointer) interface{} { return (*ReadIn)(ptr) },
		_OP_READDIR:      func(ptr unsafe.Pointer) interface{} { return (*ReadIn)(ptr) },
		_OP_ACCESS:       func(ptr unsafe.Pointer) interface{} { return (*raw.AccessIn)(ptr) },
		_OP_FORGET:       func(ptr unsafe.Pointer) interface{} { return (*raw.ForgetIn)(ptr) },
		_OP_BATCH_FORGET: func(ptr unsafe.Pointer) interface{} { return (*raw.BatchForgetIn)(ptr) },
		_OP_LINK:         func(ptr unsafe.Pointer) interface{} { return (*raw.LinkIn)(ptr) },
		_OP_MKDIR:        func(ptr unsafe.Pointer) interface{} { return (*raw.MkdirIn)(ptr) },
		_OP_RELEASE:      func(ptr unsafe.Pointer) interface{} { return (*raw.ReleaseIn)(ptr) },
		_OP_RELEASEDIR:   func(ptr unsafe.Pointer) interface{} { return (*raw.ReleaseIn)(ptr) },
	} {
		operationHandlers[op].DecodeIn = f
	}

	// File name args.
	for op, count := range map[int32]int{
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

	var r request
	sizeOfOutHeader := unsafe.Sizeof(raw.OutHeader{})
	for code, h := range operationHandlers {
		if h.OutputSize + sizeOfOutHeader > unsafe.Sizeof(r.outBuf) {
			log.Panicf("request output buffer too small: code %v, sz %d + %d %v", code, h.OutputSize, sizeOfOutHeader, h)
		}
	}
}
