// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type deviceRegion struct {
	VhostUserMemoryRegion

	Data []byte
}

func (r *deviceRegion) Close() error {
	if r.Data != nil {
		return syscall.Munmap(r.Data)
	}

	return nil
}

func (r *deviceRegion) configure(fd int, reg *VhostUserMemoryRegion) error {
	if reg.DriverAddr+reg.MemorySize < reg.DriverAddr {
		return fmt.Errorf("overflow 0x%x, sz %x", reg.DriverAddr, reg.MemorySize)
	}

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

// FromDriverAddr translates a driver (host-virtual) address to a host pointer.
func (r *deviceRegion) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	if driverAddr < r.DriverAddr || driverAddr >= r.DriverAddr+r.MemorySize {
		return nil
	}

	return unsafe.Pointer(&r.Data[driverAddr-r.DriverAddr+r.MmapOffset])
}
