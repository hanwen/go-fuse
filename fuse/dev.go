// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// Dev is an open /dev/fuse file descriptor, and the connection
// negotiated on it, if any.
type Dev struct {
	fd int

	// The INIT negotiation; zero if it has not happened yet.
	in  InitIn
	out InitOut

	// ready receives the result of mounting; see Server.WaitMount.
	ready chan error
}

// negotiated returns whether INIT was negotiated on the connection.
func (d *Dev) negotiated() bool {
	return d.out.Major != 0
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

// Dev returns a device for the connection of s, with a duplicate of
// its file descriptor, eg. to pass to another process. NewServerFromDev
// with this device serves the connection without negotiating it
// again. Typically, the server is stopped first. The caller must
// close the device, unless it is passed to NewServerFromDev.
func (s *Server) Dev() (*Dev, error) {
	var nfd int
	var err error
	if cerr := s.fuseFD.conn.Control(func(fd uintptr) {
		nfd, err = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	}); cerr != nil {
		return nil, cerr
	}
	if err != nil {
		return nil, err
	}
	ready := make(chan error)
	close(ready)
	return &Dev{
		fd:    nfd,
		in:    s.conn.in,
		out:   s.conn.out,
		ready: ready,
	}, nil
}

// Init returns the INIT negotiation of the connection. It is zero if
// the connection has not been negotiated yet.
func (d *Dev) Init() (InitIn, InitOut) {
	return d.in, d.out
}

// NewDev returns a device for the FUSE file descriptor fd, and takes
// ownership of it. in and out are the INIT negotiation from Init,
// eg. received from another process along with fd. If they are nil,
// the connection has not been negotiated yet, eg. for a device that a
// privileged process opened and mounted.
func NewDev(fd int, in *InitIn, out *InitOut) *Dev {
	ready := make(chan error)
	close(ready)
	d := &Dev{fd: fd, ready: ready}
	if in != nil {
		d.in = *in
	}
	if out != nil {
		d.out = *out
	}
	return d
}
