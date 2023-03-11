// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const FUSET_SRV_PATH = "/usr/local/bin/go-nfsv4"

var osxFuse bool

func unixgramSocketpair() (l, r *os.File, err error) {
	fd, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair",
			err.(syscall.Errno))
	}
	l = os.NewFile(uintptr(fd[0]), fmt.Sprintf("socketpair-half%d", fd[0]))
	r = os.NewFile(uintptr(fd[1]), fmt.Sprintf("socketpair-half%d", fd[1]))
	return
}

func mount(mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	if fuset_bin, err := fusetBinary(); err == nil {
		osxFuse = false
		return mount_fuset(fuset_bin, mountPoint, opts, ready)
	} else if osxfuse_bin, err := fusermountBinary(); err == nil {
		osxFuse = true
		return mount_osxfuse(osxfuse_bin, mountPoint, opts, ready)
	}
	return -1, fmt.Errorf("not FUSE-T nor osxFuse found")
}

// Declare these as globals to prevent them from being garbage collected,
// as we utilize the underlying file descriptors rather than the objects.
var local, local_mon, remote, remote_mon *os.File

// Create a FUSE FS on the specified mount point.  The returned
// mount point is always absolute.
func mount_fuset(bin string, mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	local, remote, err = unixgramSocketpair()
	if err != nil {
		return
	}
	defer remote.Close()

	local_mon, remote_mon, err = unixgramSocketpair()
	if err != nil {
		return
	}
	defer remote_mon.Close()

	args := []string{}
	if opts.Debug {
		args = append(args, "-d")
	}
	if opts.FsName != "" {
		args = append(args, "--volname")
		args = append(args, opts.FsName)
	}
	for _, opts := range opts.optionsStrings() {
		if opts == "ro" {
			args = append(args, "-r")
		}
	}

	args = append(args, fmt.Sprintf("--rwsize=%d", opts.MaxWrite))
	args = append(args, mountPoint)
	cmd := exec.Command(bin, args...)
	cmd.ExtraFiles = []*os.File{remote, remote_mon} // fd would be (index + 3)

	envs := []string{}
	envs = append(envs, "_FUSE_COMMFD=3")
	envs = append(envs, "_FUSE_MONFD=4")
	envs = append(envs, "_FUSE_COMMVERS=2")
	cmd.Env = append(os.Environ(), envs...)

	syscall.CloseOnExec(int(local.Fd()))
	syscall.CloseOnExec(int(local_mon.Fd()))

	if err = cmd.Start(); err != nil {
		return
	}
	cmd.Process.Release()
	fd = int(local.Fd())
	go func() {
		if _, err = local_mon.Write([]byte("mount")); err != nil {
			err = fmt.Errorf("fuse-t failed: %v", err)
		} else {
			reply := make([]byte, 4)
			if _, err = local_mon.Read(reply); err != nil {
				fmt.Printf("mount read  %v\n", err)
				err = fmt.Errorf("fuse-t failed: %v", err)
			}
		}

		ready <- err
		close(ready)
	}()

	return fd, err
}

// Create a FUSE FS on the specified mount point.  The returned
// mount point is always absolute.
func mount_osxfuse(bin string, mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	local, remote, err := unixgramSocketpair()
	if err != nil {
		return
	}

	defer local.Close()
	defer remote.Close()

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

	if message.Type != syscall.SCM_RIGHTS {
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

func fusetBinary() (string, error) {
	srv_path := os.Getenv("FUSE_NFSSRV_PATH")
	if srv_path == "" {
		srv_path = FUSET_SRV_PATH
	}

	if _, err := os.Stat(srv_path); err == nil {
		return srv_path, nil
	}

	return "", fmt.Errorf("FUSE-T not found")
}
