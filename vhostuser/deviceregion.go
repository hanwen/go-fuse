// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type deviceRegion struct {
	VhostUserMemoryRegion

	Data []byte
}

func (r *deviceRegion) configure(fd int, reg *VhostUserMemoryRegion) error {
	data, err := syscall.Mmap(fd, int64(reg.MmapOffset), int(reg.MemorySize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_NORESERVE)
	if err != nil {
		return err
	}
	syscall.Madvise(data, unix.MADV_DONTDUMP)
	*r = deviceRegion{
		VhostUserMemoryRegion: VhostUserMemoryRegion{
			GuestPhysAddr: reg.GuestPhysAddr,
			MemorySize:    reg.MemorySize,
			DriverAddr:    reg.DriverAddr,
			MmapOffset:    0, // input holds the offset into the fd.
		},
		Data: data,
	}
	return nil
}

func (r *deviceRegion) String() string {
	return r.VhostUserMemoryRegion.String()
}

func (r *deviceRegion) containsGuestAddr(guestAddr uint64) bool {
	return guestAddr >= r.GuestPhysAddr && guestAddr < r.GuestPhysAddr+r.MemorySize
}

func (r *deviceRegion) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	if driverAddr < r.VhostUserMemoryRegion.DriverAddr || driverAddr >= r.DriverAddr+r.MemorySize {
		return nil
	}

	return unsafe.Pointer(&r.Data[driverAddr-r.DriverAddr+r.MmapOffset])
}
