// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"syscall"
)

// Dev is an open /dev/fuse file descriptor (struct fuse_dev in the
// kernel).
type Dev struct {
	fd int

	// ready receives the result of mounting; see Server.WaitMount.
	ready chan error
}

// Fd returns the file descriptor, eg. for syscall.UnixRights. It
// remains owned by d.
func (d *Dev) Fd() int {
	return d.fd
}

// Close closes the file descriptor. If it is the last one for the
// mount, the kernel aborts the connection.
func (d *Dev) Close() error {
	if d.fd < 0 {
		return nil
	}
	fd := d.fd
	d.fd = -1
	return syscall.Close(fd)
}

// release transfers ownership of the fd to the caller.
func (d *Dev) release() int {
	fd := d.fd
	d.fd = -1
	return fd
}
