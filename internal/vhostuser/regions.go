// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"
)

type deviceRegions struct {
	mu sync.Mutex
	// sorted by GuestPhysAddr; updated atomically so readers need no lock.
	regions atomic.Pointer[[]deviceRegion]
}

func (d *deviceRegions) load() []deviceRegion {
	if p := d.regions.Load(); p != nil {
		return *p
	}
	return nil
}

func (d *deviceRegions) dumpRegions() {
	regs := d.load()
	for i := range regs {
		log.Printf("region %d: %v", i, &regs[i])
	}
}

func (d *deviceRegions) Close() error {
	var retErr error
	for _, r := range d.load() {
		if err := r.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}
	return retErr
}

func (d *deviceRegions) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	for _, r := range d.load() {
		p := r.FromDriverAddr(driverAddr)
		if p != nil {
			return p
		}
	}
	return nil
}

func (d *deviceRegions) FromGuestAddr(guestAddr uint64, sz uint64) []byte {
	regs := d.load()
	idx := findRegionByGuestAddr(regs, guestAddr)
	if idx >= len(regs) {
		return nil
	}
	r := regs[idx]
	if !r.containsGuestAddr(guestAddr) {
		return nil
	}

	seg := r.Data[guestAddr-r.GuestPhysAddr:]
	if len(seg) > int(sz) {
		seg = seg[:sz]
	}
	return seg
}

// findRegionByGuestAddr returns the index of the region that may contain
// guestAddr.  The caller must check containsGuestAddr on the result.
func findRegionByGuestAddr(regs []deviceRegion, guestAddr uint64) int {
	return sort.Search(len(regs),
		func(i int) bool {
			return guestAddr < regs[i].GuestPhysAddr+regs[i].MemorySize
		})
}

func (d *deviceRegions) AddMemReg(fd int, reg *VhostUserMemoryRegion) error {
	if hps := getFDHugepagesize(fd); hps != 0 {
		return fmt.Errorf("huge pages")
	}

	var dr deviceRegion
	if err := dr.configure(fd, reg); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	old := d.load()
	if len(old) == int(d.GetMaxMemslots()) {
		return fmt.Errorf("out of memory slots")
	}

	idx := findRegionByGuestAddr(old, reg.GuestPhysAddr)
	newRegs := make([]deviceRegion, len(old)+1)
	copy(newRegs, old[:idx])
	newRegs[idx] = dr
	copy(newRegs[idx+1:], old[idx:])
	d.regions.Store(&newRegs)
	return nil
}

// MAX_MEM_SLOTS is the maximum number of memory regions the device accepts.
// 509 matches QEMU's VHOST_USER_MAX_RAM_SLOTS; when CONFIGURE_MEM_SLOTS is
// negotiated the limit is communicated to the driver via GET_MAX_MEM_SLOTS.
const MAX_MEM_SLOTS = 509

func (d *deviceRegions) GetMaxMemslots() uint64 {
	return MAX_MEM_SLOTS
}
