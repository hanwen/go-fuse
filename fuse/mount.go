package fuse

// Written with a look to http://ptspts.blogspot.com/2009/11/fuse-protocol-tutorial-for-linux-26.html

import (
	"fmt"
	"net"
	"os"
	"path"
	"syscall"
	"unsafe"
)

const (
	bufSize = 66000
)

type mounted string

// Mount create a fuse fs on the specified mount point.
func mount(mountPoint string) (f *os.File, m mounted, err os.Error) {
	local, remote, err := net.Socketpair("unixgram")
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	mountPoint = path.Clean(mountPoint)
	if !path.Rooted(mountPoint) {
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
		[]*os.File{nil, nil, nil, remote.File()})
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

func recvmsg(fd int, msg *syscall.Msghdr, flags int) (n int, errno int) {
	n1, _, e1 := syscall.Syscall(syscall.SYS_RECVMSG, uintptr(fd), uintptr(unsafe.Pointer(msg)), uintptr(flags))
	n = int(n1)
	errno = int(e1)
	return
}

func Recvmsg(fd int, msg *syscall.Msghdr, flags int) (n int, err os.Error) {
	n, errno := recvmsg(fd, msg, flags)
	if n == 0 && errno == 0 {
		return 0, os.EOF
	}
	if errno != 0 {
		err = os.NewSyscallError("recvmsg", errno)
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
		iovecs[i].Len = uint64(len(packet[i]))
	}
	n, errno := writev(fd, (*syscall.Iovec)(unsafe.Pointer(&iovecs[0])), len(iovecs))

	if errno != 0 {
		err = os.NewSyscallError("writev", errno)
		return
	}
	return
}

func getFuseConn(local net.Conn) (f *os.File, err os.Error) {
	var msg syscall.Msghdr
	var iov syscall.Iovec
	base := make([]int32, 256)
	control := make([]int32, 256)

	iov.Base = (*byte)(unsafe.Pointer(&base[0]))
	iov.Len = uint64(len(base) * 4)
	msg.Iov = (*syscall.Iovec)(unsafe.Pointer(&iov))
	msg.Iovlen = 1
	msg.Control = (*byte)(unsafe.Pointer(&control[0]))
	msg.Controllen = uint64(len(control) * 4)

	_, err = Recvmsg(local.File().Fd(), &msg, 0)
	if err != nil {
		return
	}

	length := control[0]
	typ := control[2] // syscall.Cmsghdr.Type
	fd := control[4]
	if typ != 1 {
		err = os.NewError(fmt.Sprintf("getFuseConn: recvmsg returned wrong control type: %d", typ))
		return
	}
	if length < 20 {
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
