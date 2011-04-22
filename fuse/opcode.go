package fuse

import (
	"fmt"
	"os"
	"unsafe"
)

func (code Status) String() string {
	if code == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", int(code), os.Errno(code))
}

func replyString(opcode Opcode, ptr unsafe.Pointer) string {
	var val interface{}
	switch opcode {
	case FUSE_LOOKUP:
		val = (*EntryOut)(ptr)
	case FUSE_OPEN:
		val = (*OpenOut)(ptr)
	}
	if val != nil {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

////////////////////////////////////////////////////////////////

func doOpen(state *MountState, req *request) {
	flags, handle, status := state.fileSystem.Open(req.inHeader, (*OpenIn)(req.inData))
	req.status = status
	if status != OK {
		return 
	}

	out := &OpenOut{
		Fh: handle,
		OpenFlags: flags,
	}

	req.data = unsafe.Pointer(out)
}


func doCreate(state *MountState, req *request) {
	flags, handle, entry, status := state.fileSystem.Create(req.inHeader, (*CreateIn)(req.inData), req.filename())
	req.status = status 
	if status == OK {
		req.data = unsafe.Pointer(&CreateOut{
			EntryOut: *entry,
			OpenOut: OpenOut{
				Fh: handle,
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
		req.data = unsafe.Pointer(&OpenOut{
			Fh: handle,
			OpenFlags: flags,
		})
	}
}

func doSetattr(state *MountState, req *request) {
	// TODO - if Fh != 0, we should do a FSetAttr instead.
	o, s := state.fileSystem.SetAttr(req.inHeader, (*SetAttrIn)(req.inData))
	req.data = unsafe.Pointer(o)
	req.status = s
}

func doWrite(state *MountState, req *request) {
	n, status := state.fileSystem.Write((*WriteIn)(req.inData), req.arg)
	o := &WriteOut{
		Size: n,
	}
	req.data = unsafe.Pointer(o)
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
		req.data = unsafe.Pointer(out)
	}

	if size > input.Size {
		req.status = ERANGE
	}

	req.flatData = data
}

////////////////////////////////////////////////////////////////

var operationNames []string
var inputSizeMap []int
var outputSizeMap []int

type operation func(*MountState, *request)
var operationFuncs []operation

func operationName(opcode Opcode) string {
	if opcode > OPCODE_COUNT {
		return "unknown"
	}
	return operationNames[opcode]
}

func inputSize(o Opcode) (int, bool) {
	return lookupSize(o, inputSizeMap)
}

func outputSize(o Opcode) (int, bool) {
	return lookupSize(o, outputSizeMap)
}

func lookupSize(o Opcode, sMap []int) (int, bool) {
	if o >= OPCODE_COUNT {
		return -1, false
	}
	return sMap[int(o)], true
}

func lookupOperation(o Opcode) operation {
	return operationFuncs[o]
}

func makeSizeMap(dict map[int]int) []int {
	out := make([]int, OPCODE_COUNT)
	for i, _ := range out {
		out[i] = -1
	}

	for code, val := range dict {
		out[code] = val
	}
	return out
}

func init() {
	inputSizeMap = makeSizeMap(map[int]int{
		FUSE_LOOKUP:      0,
		FUSE_FORGET:      unsafe.Sizeof(ForgetIn{}),
		FUSE_GETATTR:     unsafe.Sizeof(GetAttrIn{}),
		FUSE_SETATTR:     unsafe.Sizeof(SetAttrIn{}),
		FUSE_READLINK:    0,
		FUSE_SYMLINK:     0,
		FUSE_MKNOD:       unsafe.Sizeof(MknodIn{}),
		FUSE_MKDIR:       unsafe.Sizeof(MkdirIn{}),
		FUSE_UNLINK:      0,
		FUSE_RMDIR:       0,
		FUSE_RENAME:      unsafe.Sizeof(RenameIn{}),
		FUSE_LINK:        unsafe.Sizeof(LinkIn{}),
		FUSE_OPEN:        unsafe.Sizeof(OpenIn{}),
		FUSE_READ:        unsafe.Sizeof(ReadIn{}),
		FUSE_WRITE:       unsafe.Sizeof(WriteIn{}),
		FUSE_STATFS:      0,
		FUSE_RELEASE:     unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNC:       unsafe.Sizeof(FsyncIn{}),
		FUSE_SETXATTR:    unsafe.Sizeof(SetXAttrIn{}),
		FUSE_GETXATTR:    unsafe.Sizeof(GetXAttrIn{}),
		FUSE_LISTXATTR:   unsafe.Sizeof(GetXAttrIn{}),
		FUSE_REMOVEXATTR: 0,
		FUSE_FLUSH:       unsafe.Sizeof(FlushIn{}),
		FUSE_INIT:        unsafe.Sizeof(InitIn{}),
		FUSE_OPENDIR:     unsafe.Sizeof(OpenIn{}),
		FUSE_READDIR:     unsafe.Sizeof(ReadIn{}),
		FUSE_RELEASEDIR:  unsafe.Sizeof(ReleaseIn{}),
		FUSE_FSYNCDIR:    unsafe.Sizeof(FsyncIn{}),
		FUSE_GETLK:       0,
		FUSE_SETLK:       0,
		FUSE_SETLKW:      0,
		FUSE_ACCESS:      unsafe.Sizeof(AccessIn{}),
		FUSE_CREATE:      unsafe.Sizeof(CreateIn{}),
		FUSE_INTERRUPT:   unsafe.Sizeof(InterruptIn{}),
		FUSE_BMAP:        unsafe.Sizeof(BmapIn{}),
		FUSE_DESTROY:     0,
		FUSE_IOCTL:       unsafe.Sizeof(IoctlIn{}),
		FUSE_POLL:        unsafe.Sizeof(PollIn{}),
	})

	outputSizeMap = makeSizeMap(map[int]int{
		FUSE_LOOKUP:      unsafe.Sizeof(EntryOut{}),
		FUSE_FORGET:      0,
		FUSE_GETATTR:     unsafe.Sizeof(AttrOut{}),
		FUSE_SETATTR:     unsafe.Sizeof(AttrOut{}),
		FUSE_READLINK:    0,
		FUSE_SYMLINK:     unsafe.Sizeof(EntryOut{}),
		FUSE_MKNOD:       unsafe.Sizeof(EntryOut{}),
		FUSE_MKDIR:       unsafe.Sizeof(EntryOut{}),
		FUSE_UNLINK:      0,
		FUSE_RMDIR:       0,
		FUSE_RENAME:      0,
		FUSE_LINK:        unsafe.Sizeof(EntryOut{}),
		FUSE_OPEN:        unsafe.Sizeof(OpenOut{}),
		FUSE_READ:        0,
		FUSE_WRITE:       unsafe.Sizeof(WriteOut{}),
		FUSE_STATFS:      unsafe.Sizeof(StatfsOut{}),
		FUSE_RELEASE:     0,
		FUSE_FSYNC:       0,
		FUSE_SETXATTR:    0,
		FUSE_GETXATTR:    unsafe.Sizeof(GetXAttrOut{}),
		FUSE_LISTXATTR:   unsafe.Sizeof(GetXAttrOut{}),
		FUSE_REMOVEXATTR: 0,
		FUSE_FLUSH:       0,
		FUSE_INIT:        unsafe.Sizeof(InitOut{}),
		FUSE_OPENDIR:     unsafe.Sizeof(OpenOut{}),
		FUSE_READDIR:     0,
		FUSE_RELEASEDIR:  0,
		FUSE_FSYNCDIR:    0,
		// TODO
		FUSE_GETLK:     0,
		FUSE_SETLK:     0,
		FUSE_SETLKW:    0,
		FUSE_ACCESS:    0,
		FUSE_CREATE:    unsafe.Sizeof(CreateOut{}),
		FUSE_INTERRUPT: 0,
		FUSE_BMAP:      unsafe.Sizeof(BmapOut{}),
		FUSE_DESTROY:   0,
		FUSE_IOCTL:     unsafe.Sizeof(IoctlOut{}),
		FUSE_POLL:      unsafe.Sizeof(PollOut{}),
	})

	operationNames = make([]string, OPCODE_COUNT)
	for k, v := range map[int]string{
		FUSE_LOOKUP:"FUSE_LOOKUP",
		FUSE_FORGET:"FUSE_FORGET",
		FUSE_GETATTR:"FUSE_GETATTR",
		FUSE_SETATTR:"FUSE_SETATTR",
		FUSE_READLINK:"FUSE_READLINK",
		FUSE_SYMLINK:"FUSE_SYMLINK",
		FUSE_MKNOD:"FUSE_MKNOD",
		FUSE_MKDIR:"FUSE_MKDIR",
		FUSE_UNLINK:"FUSE_UNLINK",
		FUSE_RMDIR:"FUSE_RMDIR",
		FUSE_RENAME:"FUSE_RENAME",
		FUSE_LINK:"FUSE_LINK",
		FUSE_OPEN:"FUSE_OPEN",
		FUSE_READ:"FUSE_READ",
		FUSE_WRITE:"FUSE_WRITE",
		FUSE_STATFS:"FUSE_STATFS",
		FUSE_RELEASE:"FUSE_RELEASE",
		FUSE_FSYNC:"FUSE_FSYNC",
		FUSE_SETXATTR:"FUSE_SETXATTR",
		FUSE_GETXATTR:"FUSE_GETXATTR",
		FUSE_LISTXATTR:"FUSE_LISTXATTR",
		FUSE_REMOVEXATTR:"FUSE_REMOVEXATTR",
		FUSE_FLUSH:"FUSE_FLUSH",
		FUSE_INIT:"FUSE_INIT",
		FUSE_OPENDIR:"FUSE_OPENDIR",
		FUSE_READDIR:"FUSE_READDIR",
		FUSE_RELEASEDIR:"FUSE_RELEASEDIR",
		FUSE_FSYNCDIR:"FUSE_FSYNCDIR",
		FUSE_GETLK:"FUSE_GETLK",
		FUSE_SETLK:"FUSE_SETLK",
		FUSE_SETLKW:"FUSE_SETLKW",
		FUSE_ACCESS:"FUSE_ACCESS",
		FUSE_CREATE:"FUSE_CREATE",
		FUSE_INTERRUPT:"FUSE_INTERRUPT",
		FUSE_BMAP:"FUSE_BMAP",
		FUSE_DESTROY:"FUSE_DESTROY",
		FUSE_IOCTL:"FUSE_IOCTL",
		FUSE_POLL:"FUSE_POLL"} {
		operationNames[k] = v
	}

	operationFuncs = make([]operation, OPCODE_COUNT)
	for k, v := range map[Opcode]operation{
	FUSE_OPEN: doOpen,
	FUSE_READDIR: doReadDir,
	FUSE_WRITE: doWrite,
	FUSE_OPENDIR: doOpenDir,
	FUSE_CREATE: doCreate,
	FUSE_SETATTR: doSetattr,
	FUSE_GETXATTR: doGetXAttr,
	FUSE_LISTXATTR:  doGetXAttr,
	} {
		operationFuncs[k] = v
	}
}
