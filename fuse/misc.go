// Random odds and ends. 

package fuse

import (
	"bytes"
	"encoding/binary"
	"rand"
	"os"
	"time"
	"fmt"
	"path"
)

// Make a temporary directory securely.
//
// Should move somewhere into Go library?
func MakeTempDir() string {
	source := rand.NewSource(time.Nanoseconds())
	number := source.Int63() & 0xffff
	name := fmt.Sprintf("tmp%d", number)

	fullName := path.Join(os.TempDir(), name)
	err := os.Mkdir(fullName, 0700)
	if err != nil {
		panic("Mkdir() should always succeed: " + fullName)
	}
	return fullName
}

// Convert os.Error back to Errno based errors.
func OsErrorToFuseError(err os.Error) Status {
	if err != nil {
		asErrno, ok := err.(os.Errno)
		if ok {
			return Status(asErrno)
		}

		asSyscallErr, ok := err.(*os.SyscallError)
		if ok {
			return Status(asSyscallErr.Errno)
		}

		// Should not happen.  Should we log an error somewhere?
		return ENOSYS
	}
	return OK
}

func operationName(opcode uint32) string {
	switch opcode {
	case FUSE_LOOKUP:
		return "FUSE_LOOKUP"
	case FUSE_FORGET:
		return "FUSE_FORGET"
	case FUSE_GETATTR:
		return "FUSE_GETATTR"
	case FUSE_SETATTR:
		return "FUSE_SETATTR"
	case FUSE_READLINK:
		return "FUSE_READLINK"
	case FUSE_SYMLINK:
		return "FUSE_SYMLINK"
	case FUSE_MKNOD:
		return "FUSE_MKNOD"
	case FUSE_MKDIR:
		return "FUSE_MKDIR"
	case FUSE_UNLINK:
		return "FUSE_UNLINK"
	case FUSE_RMDIR:
		return "FUSE_RMDIR"
	case FUSE_RENAME:
		return "FUSE_RENAME"
	case FUSE_LINK:
		return "FUSE_LINK"
	case FUSE_OPEN:
		return "FUSE_OPEN"
	case FUSE_READ:
		return "FUSE_READ"
	case FUSE_WRITE:
		return "FUSE_WRITE"
	case FUSE_STATFS:
		return "FUSE_STATFS"
	case FUSE_RELEASE:
		return "FUSE_RELEASE"
	case FUSE_FSYNC:
		return "FUSE_FSYNC"
	case FUSE_SETXATTR:
		return "FUSE_SETXATTR"
	case FUSE_GETXATTR:
		return "FUSE_GETXATTR"
	case FUSE_LISTXATTR:
		return "FUSE_LISTXATTR"
	case FUSE_REMOVEXATTR:
		return "FUSE_REMOVEXATTR"
	case FUSE_FLUSH:
		return "FUSE_FLUSH"
	case FUSE_INIT:
		return "FUSE_INIT"
	case FUSE_OPENDIR:
		return "FUSE_OPENDIR"
	case FUSE_READDIR:
		return "FUSE_READDIR"
	case FUSE_RELEASEDIR:
		return "FUSE_RELEASEDIR"
	case FUSE_FSYNCDIR:
		return "FUSE_FSYNCDIR"
	case FUSE_GETLK:
		return "FUSE_GETLK"
	case FUSE_SETLK:
		return "FUSE_SETLK"
	case FUSE_SETLKW:
		return "FUSE_SETLKW"
	case FUSE_ACCESS:
		return "FUSE_ACCESS"
	case FUSE_CREATE:
		return "FUSE_CREATE"
	case FUSE_INTERRUPT:
		return "FUSE_INTERRUPT"
	case FUSE_BMAP:
		return "FUSE_BMAP"
	case FUSE_DESTROY:
		return "FUSE_DESTROY"
	case FUSE_IOCTL:
		return "FUSE_IOCTL"
	case FUSE_POLL:
		return "FUSE_POLL"
	}
	return "UNKNOWN"
}

func errorString(code Status) string {
	if code == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", code, os.Errno(code))
}

func newInput(opcode uint32) Empty {
	switch opcode {
	case FUSE_FORGET:
		return new(ForgetIn)
	case FUSE_GETATTR:
		return new(GetAttrIn)
	case FUSE_MKNOD:
		return new(MknodIn)
	case FUSE_MKDIR:
		return new(MkdirIn)
	case FUSE_RENAME:
		return new(RenameIn)
	case FUSE_LINK:
		return new(LinkIn)
	case FUSE_SETATTR:
		return new(SetAttrIn)
	case FUSE_OPEN:
		return new(OpenIn)
	case FUSE_CREATE:
		return new(CreateIn)
	case FUSE_FLUSH:
		return new(FlushIn)
	case FUSE_RELEASE:
		return new(ReleaseIn)
	case FUSE_READ:
		return new(ReadIn)
	case FUSE_WRITE:
		return new(WriteIn)
	case FUSE_FSYNC:
		return new(FsyncIn)
	// case FUSE_GET/SETLK(W)
	case FUSE_ACCESS:
		return new(AccessIn)
	case FUSE_INIT:
		return new(InitIn)
	case FUSE_BMAP:
		return new(BmapIn)
	case FUSE_INTERRUPT:
		return new(InterruptIn)
	case FUSE_IOCTL:
		return new(IoctlIn)
	case FUSE_POLL:
		return new(PollIn)
	case FUSE_SETXATTR:
		return new(SetXAttrIn)
	case FUSE_GETXATTR:
		return new(GetXAttrIn)
	case FUSE_OPENDIR:
		return new(OpenIn)
	case FUSE_FSYNCDIR:
		return new(FsyncIn)
	case FUSE_READDIR:
		return new(ReadIn)
	case FUSE_RELEASEDIR:
		return new(ReleaseIn)

	}
	return nil
}

func parseLittleEndian(b *bytes.Buffer, data interface{}) bool {
	err := binary.Read(b, binary.LittleEndian, data)
	if err == nil {
		return true
	}
	if err == os.EOF {
		return false
	}
	panic(fmt.Sprintf("Cannot parse %v", data))
}
