// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fusefs_args from <sys/mount.h>.
type fusefsArgs struct {
	Name       *byte
	Fd         int32
	MaxRead    int32
	AllowOther int32
	_          int32
}

// maxFuseDevices bounds the /dev/fuseN units we try. Each unit
// serves one mount at a time.
const maxFuseDevices = 16

// fuseBufMaxSize is FUSEBUFMAXSIZE from <sys/fusebuf.h>.
const fuseBufMaxSize = 4096 * 1024

func getMaxWrite() int {
	return 1 << 20
}

// openFuseDevice opens the first free /dev/fuseN. fuse(4) refuses a
// second open of a unit with EBUSY.
func openFuseDevice() (int, error) {
	var firstErr error
	for i := 0; i < maxFuseDevices; i++ {
		dev := fmt.Sprintf("/dev/fuse%d", i)
		fd, err := syscall.Open(dev, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
		if err == nil {
			return fd, nil
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("open %s: %w", dev, err)
		}
		if errors.Is(err, syscall.ENOENT) && i > 0 {
			break
		}
	}
	return -1, firstErr
}

func mountFlags(opts *MountOptions) int {
	flags := 0
	for _, o := range opts.Options {
		switch o {
		case "ro":
			flags |= unix.MNT_RDONLY
		case "rw":
			flags &^= unix.MNT_RDONLY
		case "noatime":
			flags |= unix.MNT_NOATIME
		case "nosuid":
			flags |= unix.MNT_NOSUID
		case "nodev":
			flags |= unix.MNT_NODEV
		case "noexec":
			flags |= unix.MNT_NOEXEC
		case "sync":
			flags |= unix.MNT_SYNCHRONOUS
		case "async":
			flags |= unix.MNT_ASYNC
		default:
			// fuse(4) has no counterpart for options like
			// default_permissions, which the kernel always
			// applies, or subtype. Ignore them like
			// libfuse does.
			if opts.Debug {
				opts.Logger.Printf("mount: ignoring option %q", o)
			}
		}
	}
	return flags
}

func mount(mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	// Note: opts.DirectMount is implied; OpenBSD has no mount helper.
	fd = parseFuseFd(mountPoint)
	if fd >= 0 {
		if opts.Debug {
			opts.Logger.Printf("mount: magic mountpoint %q, using fd %d", mountPoint, fd)
		}
		syscall.CloseOnExec(fd)
		close(ready)
		return fd, nil
	}

	flags := mountFlags(opts)
	fd, err = openFuseDevice()
	if err != nil {
		return -1, err
	}

	name := opts.FsName
	if name == "" {
		name = opts.Name
	}
	if name == "" {
		name = "fusefs"
	}
	nameBytes, err := syscall.BytePtrFromString(name)
	if err != nil {
		syscall.Close(fd)
		return -1, err
	}
	maxRead := opts.MaxWrite
	if maxRead <= 0 || maxRead > fuseBufMaxSize {
		maxRead = fuseBufMaxSize
	}
	args := fusefsArgs{
		Name:    nameBytes,
		Fd:      int32(fd),
		MaxRead: int32(maxRead),
	}
	if opts.AllowOther {
		args.AllowOther = 1
	}

	if opts.Debug {
		opts.Logger.Printf("mount: fuse at %q, fd %d, flags %#x, options %s", mountPoint, fd, flags,
			strings.Join(opts.Options, ","))
	}
	if err := unix.Mount("fuse", mountPoint, flags, unsafe.Pointer(&args)); err != nil {
		syscall.Close(fd)
		return -1, &os.PathError{Op: "mount", Path: mountPoint, Err: err}
	}

	// The kernel queues FUSE_INIT during mount(2), so the mount is
	// usable as soon as the server reads from fd.
	close(ready)
	return fd, nil
}

func unmount(mountPoint string) error {
	return unix.Unmount(mountPoint, 0)
}
