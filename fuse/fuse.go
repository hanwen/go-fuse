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

type FileSystem interface {
}

type MountPoint struct {
	mountPoint string
	f *os.File
}

// Mount create a fuse fs on the specified mount point.
func Mount(mountPoint string, fs FileSystem) (m *MountPoint, err os.Error) {
	local, remote, err := net.Socketpair("unixgram")
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	fmt.Printf("Mount: 20\n")
	mountPoint = path.Clean(mountPoint)
	if !path.Rooted(mountPoint) {
		cwd, err := os.Getwd()
		if err != nil {
			return
		}
		mountPoint = path.Clean(path.Join(cwd, mountPoint))
	}
	fmt.Printf("Mount: 40\n")
	pid, err := os.ForkExec("/bin/fusermount",
			[]string { "/bin/fusermount", mountPoint },
			[]string { "_FUSE_COMMFD=3" },
			"",
			[]*os.File { nil, nil, os.Stderr, remote.File() })
	if err != nil {
		return
	}
	w, err := os.Wait(pid, 0)
	if err != nil {
		return
	}
	if w.ExitStatus() != 0 {
		return nil, os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
	}
	fmt.Printf("Mount: 100\n")
	f, err := getFuseConn(local)
	fmt.Printf("Mount: 110\n")
	if err != nil {
		return
	}
	m = &MountPoint { mountPoint, f }
	fmt.Printf("I'm here!!\n")
	return
}

func (m *MountPoint) Unmount() (err os.Error) {
	if m == nil {
		return nil
	}
	pid, err := os.ForkExec("/bin/fusermount",
			[]string { "/bin/fusermount", "-u", m.mountPoint },
			nil,
			"",
			[]*os.File { nil, nil, os.Stderr })
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
	m.f.Close()
	return
}

func recvmsg(fd int, msg *msghdr, flags int) (n int, errno int) {
	n1, _, e1 := syscall.Syscall(syscall.SYS_RECVMSG, uintptr(fd), uintptr(unsafe.Pointer(msg)), uintptr(flags))
	n = int(n1)
	errno = int(e1)
	return
}

type msghdr struct {
	Name       uintptr
	Namelen    uintptr
	Iov        uintptr
	Iovlen     uintptr
	Control    uintptr
	Controllen uintptr
	Flags      uintptr
}

func Recvmsg(fd int, msg *msghdr, flags int) (n int, err os.Error) {
	fmt.Printf("Recvmsg, 0\n")
	n, errno := recvmsg(fd, msg, flags)
	fmt.Printf("Recvmsg, 10\n")
	if errno != 0 {
		err = os.NewSyscallError("recvmsg", errno)
	}
	return
}

func getFuseConn(local net.Conn) (f * os.File, err os.Error) {
	var msg msghdr
	var iov syscall.Iovec
	base := make([]int32, 256)
	control := make([]int32, 256)

	iov.Base = (*byte)(unsafe.Pointer(&base[0]))
	iov.Len = uint64(len(base) * 4)
	msg.Iov = uintptr(unsafe.Pointer(&iov))
	msg.Iovlen = 1
	msg.Control = uintptr(unsafe.Pointer(&control[0]))
	msg.Controllen = uintptr(len(control) * 4)

	_, err = Recvmsg(local.File().Fd(), &msg, 0)
	fmt.Printf("getFuseConn: 100\n")
	if err != nil {
		return
	}
	fmt.Printf("getFuseConn: 110\n")

	length := control[0]
	fmt.Printf("getFuseConn: 120\n")
	typ := control[2] // syscall.Cmsghdr.Type
	fd := control[4]
	fmt.Printf("getFuseConn: 130\n")
	if typ != 1 {
		err = os.NewError(fmt.Sprintf("getFuseConn: recvmsg returned wrong control type: %d", typ))
		return
	}
	if length < 20 {
		err = os.NewError(fmt.Sprintf("getFuseConn: too short control message. Length: %d", length))
		return
	}

	if (fd < 0) {
		err = os.NewError(fmt.Sprintf("getFuseConn: fd < 0: %d", fd))
		return
	}
	fmt.Printf("fd: %d\n", fd)

	fmt.Printf("getFuseConn: 180\n")
	f = os.NewFile(int(fd), "fuse-conn")
	return
}


