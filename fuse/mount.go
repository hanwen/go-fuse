package fuse

import (
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
func mount(mountPoint string, options string) (f *os.File, finalMountPoint string, err error) {
	local, remote, err := unixgramSocketpair()
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	mountPoint = filepath.Clean(mountPoint)
	if !filepath.IsAbs(mountPoint) {
		cwd := ""
		cwd, err = os.Getwd()
		if err != nil {
			return
		}
		mountPoint = filepath.Clean(filepath.Join(cwd, mountPoint))
	}

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

	f, err = getConnection(local)
	finalMountPoint = mountPoint
	return
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
	dir, _ := filepath.Split(mountPoint)
	proc, err := os.StartProcess(fusermountBinary,
		[]string{fusermountBinary, "-u", mountPoint},
		&os.ProcAttr{Dir: dir, Files: []*os.File{nil, nil, os.Stderr}})
	if err != nil {
		return
	}
	w, err := proc.Wait()
	if err != nil {
		return
	}
	if !w.Success() {
		return fmt.Errorf("fusermount -u exited with code %v\n", w.Sys())
	}
	return
}

func getConnection(local *os.File) (f *os.File, err error) {
	var data [4]byte
	control := make([]byte, 4*256)

	// n, oobn, recvflags, from, errno  - todo: error checking.
	_, oobn, _, _,
		err := syscall.Recvmsg(
		int(local.Fd()), data[:], control[:], 0)
	if err != nil {
		return
	}

	message := *(*syscall.Cmsghdr)(unsafe.Pointer(&control[0]))
	fd := *(*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&control[0])) + syscall.SizeofCmsghdr))

	if message.Type != 1 {
		err = fmt.Errorf("getConnection: recvmsg returned wrong control type: %d", message.Type)
		return
	}
	if oobn <= syscall.SizeofCmsghdr {
		err = fmt.Errorf("getConnection: too short control message. Length: %d", oobn)
		return
	}
	if fd < 0 {
		err = fmt.Errorf("getConnection: fd < 0: %d", fd)
		return
	}
	f = os.NewFile(uintptr(fd), "<fuseConnection>")
	return
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
