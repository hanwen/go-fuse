// Random odds and ends.

package fuse

import (
	"os"
	"fmt"
	"log"
	"math"
	"regexp"
	"syscall"
	"unsafe"
	"io/ioutil"
)


func (code Status) String() string {
	if code == OK {
		return "OK"
	}
	return fmt.Sprintf("%d=%v", int(code), os.Errno(code))
}

// Make a temporary directory securely.
func MakeTempDir() string {
	nm, err := ioutil.TempDir("", "go-fuse")
	if err != nil {
		panic("TempDir() failed: " + err.String())
	}
	return nm
}

// Convert os.Error back to Errno based errors.
func OsErrorToErrno(err os.Error) Status {
	if err != nil {
		switch t := err.(type) {
		case os.Errno:
			return Status(t)
		case *os.SyscallError:
			return Status(t.Errno)
		case *os.PathError:
			return OsErrorToErrno(t.Error)
		case *os.LinkError:
			return OsErrorToErrno(t.Error)
		default:
			log.Println("can't convert error type:", err)
			return ENOSYS
		}
	}
	return OK
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

	f, err := os.Open("/proc/stat")
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

func MyPID() string {
	v, _ := os.Readlink("/proc/self")
	return v
}
