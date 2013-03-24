package fuse

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

var fusermountBinary string
var umountBinary string

func unixgramSocketpair() (l, r *os.File, err error) {
	fd, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair",
			err.(syscall.Errno))
	}
	l = os.NewFile(uintptr(fd[0]), "socketpair-half1")
	r = os.NewFile(uintptr(fd[1]), "socketpair-half2")
	return
}

// Create a FUSE FS on the specified mount point.  The returned
// mount point is always absolute.
func mount(mountPoint string, options string) (fd int, err error) {
	local, remote, err := unixgramSocketpair()
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	cmd := []string{fusermountBinary, mountPoint}
	if options != "" {
		cmd = append(cmd, "-o")
		cmd = append(cmd, options)
	}
	proc, err := os.StartProcess(fusermountBinary,
		cmd,
		&os.ProcAttr{
			Env:   []string{"_FUSE_COMMFD=3"},
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, remote}})

	if err != nil {
		return
	}

	w, err := proc.Wait()
	if err != nil {
		return
	}
	if !w.Success() {
		err = fmt.Errorf("fusermount exited with code %v\n", w.Sys())
		return
	}

	return getConnection(local)
}

func privilegedUnmount(mountPoint string) error {
	dir, _ := filepath.Split(mountPoint)
	proc, err := os.StartProcess(umountBinary,
		[]string{umountBinary, mountPoint},
		&os.ProcAttr{Dir: dir, Files: []*os.File{nil, nil, os.Stderr}})
	if err != nil {
		return err
	}
	w, err := proc.Wait()
	if !w.Success() {
		return fmt.Errorf("umount exited with code %v\n", w.Sys())
	}
	return err
}

func unmount(mountPoint string) (err error) {
	if os.Geteuid() == 0 {
		return privilegedUnmount(mountPoint)
	}
	errBuf := bytes.Buffer{}
	cmd := exec.Command(fusermountBinary, "-u", mountPoint)
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errBuf.Len() > 0 {
		return fmt.Errorf("%s (code %v)\n",
			errBuf.String(), err)
	}
	return err
}

func getConnection(local *os.File) (int, error) {
	var data [4]byte
	control := make([]byte, 4*256)

	// n, oobn, recvflags, from, errno  - todo: error checking.
	_, oobn, _, _,
		err := syscall.Recvmsg(
		int(local.Fd()), data[:], control[:], 0)
	if err != nil {
		return 0, err
	}

	message := *(*syscall.Cmsghdr)(unsafe.Pointer(&control[0]))
	fd := *(*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&control[0])) + syscall.SizeofCmsghdr))

	if message.Type != 1 {
		return 0, fmt.Errorf("getConnection: recvmsg returned wrong control type: %d", message.Type)
	}
	if oobn <= syscall.SizeofCmsghdr {
		return 0, fmt.Errorf("getConnection: too short control message. Length: %d", oobn)
	}
	if fd < 0 {
		return 0, fmt.Errorf("getConnection: fd < 0: %d", fd)
	}
	return int(fd), nil
}

func init() {
	var err error
	fusermountBinary, err = exec.LookPath("fusermount")
	if err != nil {
		log.Fatal("Could not find fusermount binary: %v", err)
	}
	umountBinary, _ = exec.LookPath("umount")
	if err != nil {
		log.Fatalf("Could not find umount binary: %v", err)
	}
}
