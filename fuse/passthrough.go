// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"log"
	"syscall"
	"unsafe"
)

const (
	_DEV_IOC_BACKING_OPEN  = 0x4010e501
	_DEV_IOC_BACKING_CLOSE = 0x4004e502
)

type backingMap struct {
	Fd      int32
	Flags   uint32
	padding uint64
}

func (ms *Server) RegisterBackingFd(fd int) (int32, syscall.Errno) {
	m := backingMap{
		Fd: int32(fd),
	}

	ms.writeMu.Lock()
	_ = m
	a, b, ep := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ms.mountFd), uintptr(_DEV_IOC_BACKING_OPEN), uintptr(unsafe.Pointer(&m)))
	ms.writeMu.Unlock()
	log.Printf("RegisterBackingFd(%d, mount %d): %d %d %d", fd, uintptr(ms.mountFd), a, b, ep)

	return int32(a), ep
}

func (ms *Server) UnregisterBackingFd(id int32) syscall.Errno {
	ms.writeMu.Lock()
	a, b, ep := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ms.mountFd), uintptr(_DEV_IOC_BACKING_CLOSE), uintptr(unsafe.Pointer(&id)))
	ms.writeMu.Unlock()
	log.Printf("UnregisterBackingFd %d %d %d", a, b, ep)

	return ep
}
