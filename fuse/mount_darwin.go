// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func unixgramSocketpair() (l, r *os.File, err error) {
	fd, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
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
func mount(mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	local, remote, err := unixgramSocketpair()
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

	bin, err := fusermountBinary()
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(bin,
		"-o", strings.Join(opts.optionsStrings(), ","),
		"-o", fmt.Sprintf("iosize=%d", opts.MaxWrite),
		mountPoint)
	cmd.ExtraFiles = []*os.File{remote} // fd would be (index + 3)
	cmd.Env = append(os.Environ(),
		"_FUSE_CALL_BY_LIB=",
		"_FUSE_DAEMON_PATH="+os.Args[0],
		"_FUSE_COMMFD=3",
		"_FUSE_COMMVERS=2",
		"MOUNT_OSXFUSE_CALL_BY_LIB=",
		"MOUNT_OSXFUSE_DAEMON_PATH="+os.Args[0])

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err = cmd.Start(); err != nil {
		return
	}

	fd, err = getConnection(local)
	if err != nil {
		return -1, err
	}

	go func() {
		// wait inside a goroutine or otherwise it would block forever for unknown reasons
		if err := cmd.Wait(); err != nil {
			err = fmt.Errorf("mount_osxfusefs failed: %v. Stderr: %s, Stdout: %s",
				err, errOut.String(), out.String())
		}

		ready <- err
		close(ready)
	}()

	// golang sets CLOEXEC on file descriptors when they are
	// acquired through normal operations (e.g. open).
	// Buf for fd, we have to set CLOEXEC manually
	syscall.CloseOnExec(fd)

	return fd, err
}

func unmount(dir string, opts *MountOptions) error {
	return syscall.Unmount(dir, 0)
}

func getConnection(local *os.File) (int, error) {
	conn, err := net.FileConn(local)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errors.New("getConnection: connection is not a *net.UnixConn")
	}

	var data [4]byte
	control := make([]byte, 4*256)

	// n, oobn, recvflags, from, errno  - todo: error checking.
	_, oobn, _, _, err := unixConn.ReadMsgUnix(data[:], control[:])
	if err != nil {
		return 0, err
	}

	messages, err := syscall.ParseSocketControlMessage(control[:oobn])
	if err != nil {
		return 0, err
	}
	if len(messages) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 control message, got %#v", messages)
	}
	message := messages[0]

	fds, err := syscall.ParseUnixRights(&message)
	if err != nil {
		return 0, err
	}
	if len(fds) != 1 {
		return 0, fmt.Errorf("getConnection: expect 1 fd, got %#v", fds)
	}
	fd := fds[0]

	if fd < 0 {
		return 0, fmt.Errorf("getConnection: fd < 0: %d", fd)
	}
	return fd, nil
}

func fusermountBinary() (string, error) {
	binPaths := []string{
		"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse",
		"/Library/Filesystems/osxfuse.fs/Contents/Resources/mount_osxfuse",
	}

	for _, path := range binPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no FUSE mount utility found")
}
