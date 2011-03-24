// Random odds and ends.

package fuse

import (
	"os"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"syscall"
	"unsafe"
	"io/ioutil"
)

// Make a temporary directory securely.
func MakeTempDir() string {
	nm, err := ioutil.TempDir("", "go-fuse")
	if err != nil {
		panic("TempDir() failed: " + err.String())
	}
	return nm
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

		asPathErr, ok := err.(*os.PathError)
		if ok {
			return OsErrorToFuseError(asPathErr.Error)
		}

		asLinkErr, ok := err.(*os.LinkError)
		if ok {
			return OsErrorToFuseError(asLinkErr.Error)
		}

		// Should not happen.  Should we log an error somewhere?
		log.Println("can't convert error type:", err)
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

func (code Status) String() string {
	if code == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", int(code), os.Errno(code))
}

func SplitNs(time float64, secs *uint64, nsecs *uint32) {
	*nsecs = uint32(1e9 * (time - math.Trunc(time)))
	*secs = uint64(math.Trunc(time))
}

func CopyFileInfo(fi *os.FileInfo, attr *Attr) {
	attr.Ino = uint64(fi.Ino)
	attr.Size = uint64(fi.Size)
	attr.Blocks = uint64(fi.Blocks)

	attr.Atime = uint64(fi.Atime_ns / 1e9)
	attr.Atimensec = uint32(fi.Atime_ns % 1e9)

	attr.Mtime = uint64(fi.Mtime_ns / 1e9)
	attr.Mtimensec = uint32(fi.Mtime_ns % 1e9)

	attr.Ctime = uint64(fi.Ctime_ns / 1e9)
	attr.Ctimensec = uint32(fi.Ctime_ns % 1e9)

	attr.Mode = fi.Mode
	attr.Nlink = uint32(fi.Nlink)
	attr.Uid = uint32(fi.Uid)
	attr.Gid = uint32(fi.Gid)
	attr.Rdev = uint32(fi.Rdev)
	attr.Blksize = uint32(fi.Blksize)
}


func writev(fd int, iovecs *syscall.Iovec, cnt int) (n int, errno int) {
	n1, _, e1 := syscall.Syscall(
		syscall.SYS_WRITEV,
		uintptr(fd), uintptr(unsafe.Pointer(iovecs)), uintptr(cnt))
	return int(n1), int(e1)
}

func Writev(fd int, packet [][]byte) (n int, err os.Error) {
	iovecs := make([]syscall.Iovec, 0, len(packet))

	for _, v := range packet {
		if v == nil || len(v) == 0 {
			continue
		}
		vec := syscall.Iovec{
			Base: &v[0],
		}
		vec.SetLen(len(v))
		iovecs = append(iovecs, vec)
	}

	if len(iovecs) == 0 {
		return 0, nil
	}

	n, errno := writev(fd, &iovecs[0], len(iovecs))
	if errno != 0 {
		err = os.NewSyscallError("writev", errno)
	}
	return n, err
}

func CountCpus() int {
	var contents [10240]byte

	f, err := os.Open("/proc/stat", os.O_RDONLY, 0)
	defer f.Close()
	if err != nil {
		return 1
	}
	n, _ := f.Read(contents[:])
	re, _ := regexp.Compile("\ncpu[0-9]")

	return len(re.FindAllString(string(contents[:n]), 100))
}

// Creates a return entry for a non-existent path.
func NegativeEntry(time float64) *EntryOut {
	out := new(EntryOut)
	out.NodeId = 0
	SplitNs(time, &out.EntryValid, &out.EntryValidNsec)
	return out
}

func ModeToType(mode uint32) uint32 {
	return (mode & 0170000) >> 12
}


func CheckSuccess(e os.Error) {
	if e != nil {
		panic(fmt.Sprintf("Unexpected error: %v", e))
	}
}

// For printing latency data.
func PrintMap(m map[string]float64) {
	keys := make([]string, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}

	sort.SortStrings(keys)
	for _, k := range keys {
		if m[k] > 0 {
			fmt.Println(k, m[k])
		}
	}
}

func MyPID() string {
	v, _ := os.Readlink("/proc/self")
	return v
}


var inputSizeMap map[int]int
var outputSizeMap map[int]int

func init() {
	inputSizeMap = map[int]int{
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
		FUSE_LISTXATTR:   0,
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
	}

	outputSizeMap = map[int]int{
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
		FUSE_LISTXATTR:   0,
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
	}
}
