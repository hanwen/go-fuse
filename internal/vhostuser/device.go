// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"log"
	"sync"
	"syscall"
)

type Device struct {
	reqFD int

	Debug bool

	vqs []*Virtq

	regions  deviceRegions
	logTable []byte

	handle func(*VirtqElem) int

	// dispatchMu guards all control-plane mutations (SET_VRING_*, ADD_MEM_REG,
	// etc.) against concurrent vring dequeue.  Control messages take the write
	// lock; queue threads take the read lock while draining the avail ring.
	// FUSE request processing runs without any lock (matching virtiofsd's
	// vu_dispatch_rwlock design).
	dispatchMu sync.RWMutex
}

func (d *Device) Close() error {
	var retErr error
	for i := range d.vqs {
		if err := d.vqs[i].Close(); err != nil && retErr == nil {
			retErr = err
		}
	}

	if err := d.regions.Close(); err != nil && retErr == nil {
		retErr = err
	}

	if d.logTable != nil {
		if err := syscall.Munmap(d.logTable); err != nil && retErr == nil {
			retErr = err
		}
	}
	return nil
}

// NewDevice creates a new virtio device. The handler should return
// the number of response bytes written.
func NewDevice(handle func(*VirtqElem) int) *Device {
	d := &Device{
		// TODO: queue count should be configurable; 2 is hardcoded (hiprio +
		// one request queue). GetQueueNum() reports this count to the driver,
		// so they must stay in sync.
		vqs: make([]*Virtq, 2),
	}
	for i := range d.vqs {
		d.vqs[i] = newVirtq(d)
	}

	d.handle = handle
	return d
}

// https://qemu-project.gitlab.io/qemu/interop/vhost-user.html#communication
// is incorrect regarding types.
func (d *Device) SetLogBase(fd int, log *VhostUserLog) error {
	data, err := syscall.Mmap(fd, int64(log.MmapOffset), int(log.MmapSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED) // |syscall.MAP_NORESERVE)?
	syscall.Close(fd)
	if err != nil {
		return err
	}
	if d.logTable != nil {
		syscall.Munmap(d.logTable)
	}

	d.logTable = data
	return nil
}

func (d *Device) SetVringAddr(addr *VhostVringAddr) error {
	return d.vqs[addr.Index].SetVringAddr(addr)
}

func (d *Device) SetVringNum(state *VhostVringState) {
	d.vqs[state.Index].Vring.Num = int(state.Num)
}

func (d *Device) SetVringBase(state *VhostVringState) {
	p := d.vqs[state.Index]
	p.ShadowAvailIdx = uint16(state.Num)
	p.LastAvailIdx = uint16(state.Num)
}

func (d *Device) SetVringEnable(state *VhostVringState) {
	vq := d.vqs[state.Index]

	enable := uint(state.Num) != 0
	if enable {
		vq.SetEnable(d.handle)
	} else {
		vq.SetEnable(nil)
	}
}

type VirtqElem struct {
	// this is the index into Vring.Desc
	index uint

	// read and write from our perspective. The write field is for
	// consumers (ie the file system). We return the total length
	// to the driver, which can find the memory through the vring
	// index above.
	Write [][]byte
	Read  [][]byte
}

func (d *Device) logQueueFill(vq *Virtq, elem *VirtqElem, len int) {
	// NOP, need LOG_SHMFD features
}

// set bit in dev.LogTable bitvector . the bitvector indexes 4k pages
// this lets the guest know there was a write in the page. Needs
// LOG_SHMFD feature.
func (d *Device) logWrite(address, sz uint64) {
	if d.logTable == nil || sz == 0 {
		return
	}

	// if !F_LOG_ALL return
	// mark addr in the d.LogTable bitvector.
	// kick the log fd.
}

// SetVringKick sets the kick eventfd for the virtqueue at index.
//
// This is not protected by dispatchMu.  A concurrent readLoop goroutine
// could be blocked in syscall.Read(vq.KickFD) while SetVringKick closes
// and replaces that fd — a data race on KickFD.  The code relies on the
// vhost-user protocol ordering: SET_VRING_KICK must arrive before
// SET_VRING_ENABLE, so the readLoop goroutine does not exist yet when the
// fd is first assigned.  Reassignment after enable is not expected from a
// well-behaved driver (QEMU never does it).
func (d *Device) SetVringKick(fd int, index uint64) error {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	if old := d.vqs[index].KickFD; old >= 0 {
		syscall.Close(old)
	}
	d.vqs[index].KickFD = fd

	// The kick FD is a notification channel, so it must be blocking.
	if err := syscall.SetNonblock(fd, false); err != nil {
		return err
	}
	return nil
}

// SetVringErr sets the error eventfd.
func (d *Device) SetVringErr(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}

	if old := d.vqs[index].ErrFD; old >= 0 {
		syscall.Close(old)
	}

	d.vqs[index].ErrFD = fd

	// no need to change blocking status: we only write to this fd.
}

// SetVringCall sets the call eventfd.
func (d *Device) SetVringCall(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	if old := d.vqs[index].CallFD; old >= 0 {
		syscall.Close(old)
	}
	d.vqs[index].CallFD = fd
	// no need to change blocking status: we only write to this fd.
}

func (d *Device) SetOwner() {

}

func (d *Device) SetReqFD(fd int) {
	d.reqFD = fd
}

func (d *Device) GetQueueNum() uint64 {
	return uint64(len(d.vqs))
}

func (h *Device) GetFeatures() []int {
	return []int{
		//"\0\0\0p\1\0\0\0"
		RING_F_INDIRECT_DESC,
		RING_F_EVENT_IDX,
		F_PROTOCOL_FEATURES,
		F_VERSION_1,
	}
}

func (h *Device) SetFeatures(fs []int) {

}

func (h *Device) SetProtocolFeatures([]int) {

}

func (h *Device) GetProtocolFeatures() []int {
	// not supporting VHOST_USER_PROTOCOL_F_PAGEFAULT, so no support for
	// postcopy listening.

	// NOTE: PROTOCOL_F_LOG_SHMFD is not advertised here, but SetLogBase is
	// implemented and reachable.  Either advertise the feature or remove the
	// handler.

	// ")\204\0\0\0\0\0\0"
	// x29 x84
	return []int{
		PROTOCOL_F_MQ,
		PROTOCOL_F_REPLY_ACK,
		PROTOCOL_F_BACKEND_REQ,
		PROTOCOL_F_BACKEND_SEND_FD,
		PROTOCOL_F_CONFIGURE_MEM_SLOTS,
	}
}
