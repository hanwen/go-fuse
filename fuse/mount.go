package fuse

// Written with a look to http://ptspts.blogspot.com/2009/11/fuse-protocol-tutorial-for-linux-26.html

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"unsafe"
)

// Make a type to attach the Unmount method.
type mounted string

func Socketpair(network string) (l, r *os.File, err os.Error) {
	var domain int
	var typ int
	switch network {
	default:
		panic("unknown network " + network)
	case "unix":
		domain = syscall.AF_UNIX
		typ = syscall.SOCK_STREAM
	case "unixgram":
		domain = syscall.AF_UNIX
		typ = syscall.SOCK_SEQPACKET
	}
	fd, errno := syscall.Socketpair(domain, typ, 0)
	if errno != 0 {
		return nil, nil, os.NewSyscallError("socketpair", errno)
	}
	l = os.NewFile(fd[0], "socketpair-half1")
	r = os.NewFile(fd[1], "socketpair-half2")
	return
}

	
// Mount create a fuse fs on the specified mount point.
func mount(mountPoint string) (f *os.File, m mounted, err os.Error) {
	local, remote, err := Socketpair("unixgram")
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	mountPoint = path.Clean(mountPoint)
	if !path.IsAbs(mountPoint) {
		cwd, err := os.Getwd()
		if err != nil {
			return
		}
		mountPoint = path.Clean(path.Join(cwd, mountPoint))
	}
	pid, err := os.ForkExec("/bin/fusermount",
		[]string{"/bin/fusermount", mountPoint},
		[]string{"_FUSE_COMMFD=3"},
		"",
		[]*os.File{nil, nil, nil, remote})
	if err != nil {
		return
	}
	w, err := os.Wait(pid, 0)
	if err != nil {
		return
	}
	if w.ExitStatus() != 0 {
		err = os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
		return
	}

	f, err = getFuseConn(local)
	m = mounted(mountPoint)
	return
}

func (m mounted) Unmount() (err os.Error) {
	mountPoint := string(m)
	pid, err := os.ForkExec("/bin/fusermount",
		[]string{"/bin/fusermount", "-u", mountPoint},
		nil,
		"",
		[]*os.File{nil, nil, os.Stderr})
	if err != nil {
		return
	}
	w, err := os.Wait(pid, 0)
	if err != nil {
		return
	}
	if w.ExitStatus() != 0 {
		return os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
	}
	return
}

func writev(fd int, iovecs *syscall.Iovec, cnt int) (n int, errno int) {
	n1, _, e1 := syscall.Syscall(syscall.SYS_WRITEV, uintptr(fd), uintptr(unsafe.Pointer(iovecs)), uintptr(cnt))
	n = int(n1)
	errno = int(e1)
	return
}

func Writev(fd int, packet [][]byte) (n int, err os.Error) {
	if len(packet) == 0 {
		return
	}
	iovecs := make([]syscall.Iovec, len(packet))
	for i, v := range packet {
		if v == nil {
			continue
		}
		iovecs[i].Base = (*byte)(unsafe.Pointer(&packet[i][0]))
		iovecs[i].SetLen(len(packet[i]))
	}
	n, errno := writev(fd, (*syscall.Iovec)(unsafe.Pointer(&iovecs[0])), len(iovecs))

	if errno != 0 {
		err = os.NewSyscallError("writev", errno)
		return
	}
	return
}

func getInt32(b []byte, idx int) int32 {
	ptr := (*int32)(unsafe.Pointer(&b[idx*4]))
	return *ptr
}


func getFuseConn(local *os.File) (f *os.File, err os.Error) {
	var data [4]byte
	control := make([]byte, 4*256)

	// n, oobn, recvflags - todo: error checking.
	_, oobn, _,
	  errno := syscall.Recvmsg(
		local.Fd(), data[:], control[:], 0)
	if errno != 0 {
		return
	}
	length := getInt32(control, 0)
	// 1 = level.
	typ := getInt32(control, 2) // syscall.Cmsghdr.Type

	// Ugh - is this 64-bit proof?
	fd := getInt32(control, 3)
	
	if typ != 1 {
		err = os.NewError(fmt.Sprintf("getFuseConn: recvmsg returned wrong control type: %d", typ))
		return
	}
	if oobn < 16 {
		err = os.NewError(fmt.Sprintf("getFuseConn: too short control message. Length: %d", length))
		return
	}

	if fd < 0 {
		err = os.NewError(fmt.Sprintf("getFuseConn: fd < 0: %d", fd))
		return
	}
	f = os.NewFile(int(fd), "fuse-conn")
	return
}
