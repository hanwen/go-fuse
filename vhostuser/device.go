// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"log"
	"sort"
	"syscall"
	"unsafe"
)

type Device struct {
	// TODO: implement support for talking back to the driver.
	reqFD int

	Debug bool

	// vring is the same as virtq?
	vqs []Virtq

	// sorted by GuestPhysAddr
	regions  []deviceRegion
	logTable []byte

	handle func(*VirtqElem) int
}

func NewDevice(handle func(*VirtqElem) int) *Device {
	d := &Device{
		vqs: make([]Virtq, 2),
	}
	for i := range d.vqs {
		d.vqs[i].Notification = true
	}

	d.handle = handle
	return d
}

func (d *Device) MapRing(vq *Virtq) error {
	if d := d.FromDriverAddr(vq.Addr.DescUserAddr); d == nil {
		return fmt.Errorf("could not map DescUserAddr %x", vq.Addr.DescUserAddr)
	} else {
		vq.Vring.Desc = unsafe.Slice((*VringDesc)(d), vq.Vring.Num)
	}
	if d := d.FromDriverAddr(vq.Addr.UsedUserAddr); d == nil {
		return fmt.Errorf("could not map UsedUserAddr %x",
			vq.Addr.UsedUserAddr)
	} else {
		vq.Vring.Used = (*VringUsed)(d)
		vq.Vring.UsedRing = unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num)
		//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
		vq.Vring.UsedAvailEvent = (*uint16)(unsafe.Pointer(&unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num+1)[vq.Vring.Num]))

	}

	if d := d.FromDriverAddr(vq.Addr.AvailUserAddr); d == nil {
		return fmt.Errorf("could not map AvailUserAddr %x",
			vq.Addr.AvailUserAddr)
	} else {
		vq.Vring.Avail = (*VringAvail)(d)
		vq.Vring.AvailRing = unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num)
		//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
		vq.Vring.AvailUsedEvent = &unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num+1)[vq.Vring.Num]
	}
	return nil
}

func (d *Device) FromDriverAddr(driverAddr uint64) unsafe.Pointer {
	for _, r := range d.regions {
		d := r.FromDriverAddr(driverAddr)
		if d != nil {
			return d
		}
	}
	return nil
}

func (d *Device) FromGuestAddr(guestAddr uint64, sz uint64) []byte {
	idx := d.findRegionByGuestAddr(guestAddr)
	r := d.regions[idx]
	if !r.containsGuestAddr(guestAddr) {
		return nil
	}

	seg := r.Data[guestAddr-r.GuestPhysAddr:]
	if len(seg) > int(sz) {
		seg = seg[:sz]
	}
	return seg
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
	vq := &d.vqs[addr.Index]
	vq.Addr = *addr
	vq.Vring.Flags = uint32(addr.Flags) // bitsize?

	vq.Vring.LogGuestAddr = unsafe.Slice((*byte)(unsafe.Pointer(uintptr(addr.LogGuestAddr))), 0) //

	if err := d.MapRing(vq); err != nil {
		return err
	}

	vq.UsedIdx = vq.Vring.Used.Idx // LE16toH
	if vq.LastAvailIdx != vq.UsedIdx {
		resume := true // device->processed_in_order()
		if resume {
			vq.ShadowAvailIdx = vq.UsedIdx
			vq.LastAvailIdx = vq.UsedIdx
		}
	}

	return nil
}

func (d *Device) SetVringNum(state *VhostVringState) {
	d.vqs[state.Index].Vring.Num = int(state.Num)
}

func (d *Device) SetVringBase(state *VhostVringState) {
	p := &d.vqs[state.Index]
	p.ShadowAvailIdx = uint16(state.Num)
	p.LastAvailIdx = uint16(state.Num)
}

func (d *Device) SetVringEnable(state *VhostVringState) {
	p := &d.vqs[state.Index]
	p.Enable = uint(state.Num)
	if p.Enable != 0 {
		d.kickMe(state.Index)
	} else {
		// make event loop from kickMe() exit
	}
}

func clearSlice(s []byte) {
	for i := range s {
		s[i] = 0
	}
}

func (d *Device) kickMe(idx uint32) {
	vq := &d.vqs[idx]

	go func() {
		for {
			var id [8]byte
			_, err := syscall.Read(vq.KickFD, id[:])
			data, err := d.popQueue(vq)
			if err != nil {
				log.Printf("popq: %v", err)
				continue
			}
			if data == nil {
				log.Printf("queue was empty")
				continue
			}
			for _, e := range data.Write {
				clearSlice(e)
			}

			if d.Debug {
				for i, e := range data.Read {
					log.Printf("read %d: %q (%d)", i, e, len(e))
				}
				outlens := []int{}
				for _, e := range data.Write {
					outlens = append(outlens, len(e))
				}
				log.Printf("id %d: write space: %v", data.index, outlens)
			}
			// should pass on vq as well?
			if d.handle != nil {
				n := d.handle(data)
				if d.Debug {
					for i, e := range data.Write {
						log.Printf("write %d: %q (%d)", i, e, len(e))
					}
				}
				d.pushQueue(vq, data, n)
				d.queueNotify(vq)
			}
		}
	}()
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

func (d *Device) dumpRegions() {
	for i, r := range d.regions {
		log.Printf("region %d: %v", i, &r)
	}
}

func (d *Device) popQueue(vq *Virtq) (*VirtqElem, error) {

	/* TODO: unlikely conditions */

	// dev->broken?
	// vq.vring.avail == 0
	if vq.ResubmitList != nil && vq.ResubmitNum > 0 {
		return nil, fmt.Errorf("resubmit")
	}

	if vq.queueEmpty() {
		return nil, nil
	}

	if int(vq.inuse) >= vq.Vring.Num {
		return nil, fmt.Errorf("virtq size exceeded")
	}

	// todo RMB read barrier.

	idx := int(vq.LastAvailIdx) % vq.Vring.Num

	vq.LastAvailIdx++
	head := vq.Vring.AvailRing[idx]
	if int(head) > vq.Vring.Num {
		log.Panicf("silly avail %d %d", head, vq.Vring.Num)
	}
	if vq.Vring.UsedAvailEvent != nil {
		*vq.Vring.UsedAvailEvent = vq.LastAvailIdx
	}

	// vu_queue_map_desc
	elem, err := d.queueMapDesc(vq, int(head))
	if elem == nil || err != nil {
		return nil, err
	}
	vq.inuse++
	d.queueInflightGet(vq, int(head))
	return elem, nil
}

func (d *Device) logQueueFill(vq *Virtq, elem *VirtqElem, len int) {
	// NOP, need LOG_SHMFD features
}

func (d *Device) pushQueue(vq *Virtq, elem *VirtqElem, len int) {
	// vu_queue_fill
	// > vu_log_queue_fill	// log_write for UsedRing write

	// vu_queue_fill l.3103
	idx := int(vq.UsedIdx) % vq.Vring.Num
	ue := VringUsedElement{
		ID:  uint32(elem.index),
		Len: uint32(len),
	}

	vq.Vring.UsedRing[idx] = ue
	// > vring_used_write
	// > vu_queue_inflight_pre_put(dev, vq, elem->index);
	//   only for VHOST_USER_PROTOCOL_F_INFLIGHT_SHMFD

	// vu_queue_flush
	// wmb barrier

	old := vq.UsedIdx
	new := uint16(old + 1) //  why not % num?
	vq.UsedIdx = new
	vq.Vring.Used.Idx = new
	// log write

	vq.inuse--

	// ? does this have something to do with u16 wrapping?
	if new-vq.SignaledUsed < new-old {
		vq.SignaledUsedValid = false
	}

	//vu_queue_inflight_post_put(dev, vq, elem->index);
	//	                only for VHOST_USER_PROTOCOL_F_INFLIGHT_SHMFD
}

// virtio-ring.h
func VringNeedEvent(eventIdx uint16, newIdx, old uint16) bool {
	return newIdx-eventIdx-1 < newIdx-old
}

func (d *Device) vringNotify(vq *Virtq) bool {
	// mem barrier

	// if F_NOTIFY_ON_EMPTY ...

	// if ! F_EVENT_IDX ...

	v := vq.SignaledUsedValid
	old := vq.SignaledUsed
	new := vq.UsedIdx
	vq.SignaledUsed = new
	vq.SignaledUsedValid = true
	return !v || VringNeedEvent(*vq.Vring.AvailUsedEvent, new, old)
}

func (d *Device) queueNotify(vq *Virtq) {
	if !d.vringNotify(vq) {
		log.Printf("queueNotify: skipped")
		return
	}

	// if INBAND_NOTIFICATIONS ...
	var payload [8]byte
	payload[0] = 1
	if _, err := syscall.Write(vq.CallFD, payload[:]); err != nil {
		log.Panicf("eventfd write: %v", err)
	}
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

func (d *Device) queueInflightGet(vq *Virtq, head int) {
	// VHOST_USER_PROTOCOL_F_INFLIGHT_SHMFD
	if vq.Inflight == nil {
		// always returns here
		return
	}
	vq.InflightDescs[head].counter = vq.Counter
	vq.Counter++
	vq.InflightDescs[head].inflight = 1
}

func (d *Device) queueMapDesc(vq *Virtq, head int) (*VirtqElem, error) {
	result := VirtqElem{
		index: uint(head),
	}

	descArray := vq.Vring.Desc
	desc := descArray[head]
	if desc.Flags&VRING_DESC_F_INDIRECT != 0 {
		eltSize := unsafe.Sizeof(VringDesc{})
		if (desc.Len % uint32(eltSize)) != 0 {
			return nil, fmt.Errorf("modulo size")
		}

		indirectAsBytes := d.FromGuestAddr(desc.Addr, uint64(desc.Len))
		if indirectAsBytes == nil {
			return nil, fmt.Errorf("OOB read %x %#v", desc.Addr, d.regions)
		}
		if len(indirectAsBytes) != int(desc.Len) {
			return nil, fmt.Errorf("partial read indirect desc")
		}
		n := desc.Len / uint32(eltSize)
		descArray = unsafe.Slice((*VringDesc)(unsafe.Pointer(&indirectAsBytes[0])), n)
		desc = descArray[0]
	}

	for {
		iov := d.readVringEntry(desc.Addr, desc.Len)
		if desc.Flags&VRING_DESC_F_WRITE != 0 {
			// virtqueue_map_desc
			result.Write = append(result.Write, iov...)
		} else {
			result.Read = append(result.Read, iov...)
		}
		//

		if desc.Flags&VRING_DESC_F_NEXT == 0 {
			break
		}

		head = int(desc.Next)
		// barrier

		// todo: check max

		desc = descArray[head]
	}

	return &result, nil
}

// take VIRTQUEUE_MAX_SIZE ?
func (d *Device) readVringEntry(physAddr uint64, sz uint32) [][]byte {
	var result [][]byte

	for sz > 0 {
		d := d.FromGuestAddr(physAddr, uint64(sz))
		result = append(result, d)
		sz -= uint32(len(d))
		physAddr += uint64(len(d))
	}

	return result
}

func (d *Device) findRegionByGuestAddr(guestAddr uint64) int {
	return sort.Search(len(d.regions),
		func(i int) bool {
			return guestAddr < d.regions[i].GuestPhysAddr+d.regions[i].MemorySize
		})
}

func (d *Device) AddMemReg(fd int, reg *VhostUserMemoryRegion) error {
	if len(d.regions) == int(d.GetMaxMemslots()) {
		return fmt.Errorf("hot add memory")
	}

	idx := d.findRegionByGuestAddr(reg.GuestPhysAddr)
	if hps := getFDHugepagesize(fd); hps != 0 {
		return fmt.Errorf("huge pages")
	}

	var dr deviceRegion
	if err := dr.configure(fd, reg); err != nil {
		return nil
	}
	d.regions = append(d.regions, deviceRegion{})
	copy(d.regions[idx+1:], d.regions[idx:])
	d.regions[idx] = dr
	return nil
}

func (d *Device) SetVringKick(fd int, index uint64) error {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	old := d.vqs[index].KickFD
	if old != 0 {
		syscall.Close(old)
	}
	d.vqs[index].KickFD = fd

	return syscall.SetNonblock(fd, false)
}

// todo consolidate
func (d *Device) SetVringErr(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}

	if old := d.vqs[index].ErrFD; old != 0 {
		syscall.Close(old)
	}

	d.vqs[index].ErrFD = fd
}

func (d *Device) SetVringCall(fd int, index uint64) {
	if index&(1<<8) != 0 {
		log.Panic("not supported")
	}
	if old := d.vqs[index].CallFD; old != 0 {
		syscall.Close(old)
	}
	d.vqs[index].CallFD = fd
}

func (d *Device) SetOwner() {

}

func (d *Device) SetReqFD(fd int) {
	d.reqFD = fd
}

const MAX_MEM_SLOTS = 509

func (d *Device) GetMaxMemslots() uint64 {
	return MAX_MEM_SLOTS
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
