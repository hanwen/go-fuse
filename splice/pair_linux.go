// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package splice

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func (p *Pair) Writev(bufs [][]byte) (int, error) {
	var n int
	var err error
	p.wConn.Control(func(wfd uintptr) {
		n, err = unix.Writev(int(wfd), bufs)
	})
	return n, err
}

func (p *Pair) SpliceFrom(src *Pair, sz int) (int, error) {
	var n int
	var err error
	src.rConn.Control(func(rfd uintptr) {
		p.wConn.Control(func(wfd uintptr) {
			var sn int64
			sn, err = syscall.Splice(int(rfd), nil, int(wfd), nil, sz, 0)
			n = int(sn)
		})
	})
	if err != nil {
		err = os.NewSyscallError("Splice", err)
	}
	return n, err
}

func (p *Pair) LoadFromAt(fd uintptr, sz int, off int64) (int, error) {
	var n int
	var err error
	p.wConn.Control(func(wfd uintptr) {
		var sn int64
		sn, err = syscall.Splice(int(fd), &off, int(wfd), nil, sz, 0)
		n = int(sn)
	})
	return n, err
}

func (p *Pair) LoadFrom(fd uintptr, sz int) (int, error) {
	if sz > p.size {
		return 0, fmt.Errorf("LoadFrom: not enough space %d, %d",
			sz, p.size)
	}

	var n int
	var err error
	p.wConn.Control(func(wfd uintptr) {
		var sn int64
		sn, err = syscall.Splice(int(fd), nil, int(wfd), nil, sz, 0)
		n = int(sn)
	})
	if err != nil {
		err = os.NewSyscallError("Splice load from", err)
	}
	return n, err
}

func (p *Pair) WriteTo(fd uintptr, n int) (int, error) {
	return p.WriteToFlags(fd, n, 0)
}

// WriteToFlags is WriteTo with splice(2) flags. /dev/fuse acts on SPLICE_F_MOVE
// even though the generic pipe-to-file path ignores it.
func (p *Pair) WriteToFlags(fd uintptr, n int, flags int) (int, error) {
	var m int
	var err error
	p.rConn.Control(func(rfd uintptr) {
		var sm int64
		sm, err = syscall.Splice(int(rfd), nil, int(fd), nil, n, flags)
		m = int(sm)
	})
	if err != nil {
		err = os.NewSyscallError("Splice write", err)
	}
	return m, err
}

const (
	_SPLICE_F_NONBLOCK = 0x2
	_FIONREAD          = 0x541b
)

func (p *Pair) discard() {
	for {
		var err error
		p.rConn.Control(func(rfd uintptr) {
			_, err = syscall.Splice(int(rfd), nil, devNullFD(), nil, int(p.size), _SPLICE_F_NONBLOCK)
		})
		if err != nil && err != syscall.EAGAIN {
			// This can happen if something closed our fd inadvertently (eg. double close)
			log.Panicf("splicing into /dev/null: %v", err)
		}

		var n int
		p.rConn.Control(func(rfd uintptr) {
			n, err = unix.IoctlGetInt(int(rfd), _FIONREAD)
		})
		if err != nil {
			log.Panicf("FIONREAD on pipe: %v", err)
		}
		if n == 0 {
			return
		}
	}
}
