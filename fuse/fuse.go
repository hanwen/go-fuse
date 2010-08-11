package fuse

// Written with a look to http://ptspts.blogspot.com/2009/11/fuse-protocol-tutorial-for-linux-26.html

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path"
	"syscall"
	"unsafe"
)

const (
	bufSize = 65536 + 100 // See the link above for the details
)

type FileSystem interface{}

type MountPoint struct {
	mountPoint string
	f          *os.File
}

// Mount create a fuse fs on the specified mount point.
func Mount(mountPoint string, fs FileSystem) (m *MountPoint, err os.Error) {
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
		return nil, os.NewError(fmt.Sprintf("fusermount exited with code %d\n", w.ExitStatus()))
	}

	f, err := getFuseConn(local)
	if err != nil {
		return
	}
	m = &MountPoint{mountPoint, f}
	go m.loop()
	return
}

func (m *MountPoint) loop() {
	buf := make([]byte, bufSize)
	f := m.f
	for {
		n, err := f.Read(buf)
		r := bytes.NewBuffer(buf[0:n])
		if err != nil {
			fmt.Printf("MountPoint.loop: Read failed, err: %v\n", err)
			os.Exit(1)
		}
		var h In_header
		err = binary.Read(r, binary.LittleEndian, &h)
		if err != nil {
			fmt.Printf("MountPoint.loop: binary.Read of fuse_in_header failed with err: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Here! n = %d, buf = %v, h = %v\n", n, buf[0:n], h)
		os.Exit(0)
	}
}

func (m *MountPoint) Unmount() (err os.Error) {
	if m == nil {
		return nil
	}
	pid, err := os.ForkExec("/bin/fusermount",
		[]string{"/bin/fusermount", "-u", m.mountPoint},
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
	m.f.Close()
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
	if errno != 0 {
		err = os.NewSyscallError("recvmsg", errno)
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
