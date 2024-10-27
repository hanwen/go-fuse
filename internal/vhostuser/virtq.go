// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"fmt"
	"log"
	"sync"
	"syscall"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/internal/barrier"
)

type Virtq struct {
	Vring Ring

	regions    *deviceRegions
	dispatchMu *sync.RWMutex

	// mu protects all mutable vring state: avail-side (LastAvailIdx,
	// ShadowAvailIdx, inuse), used-side (UsedIdx, SignaledUsed,
	// SignaledUsedValid), and the shared inuse counter.
	mu sync.Mutex

	Inflight      *VirtqInflight
	InflightDescs []DescStateSplit
	ResubmitList  *InflightDesc

	ResubmitNum uint16

	Counter      uint64
	LastAvailIdx uint16

	ShadowAvailIdx uint16

	UsedIdx      uint16
	SignaledUsed uint16

	SignaledUsedValid bool
	Notification      bool
	Debug             *bool

	inuse uint

	handler func(*Device, int)

	CallFD int
	KickFD int
	ErrFD  int
	Enable uint

	Addr VhostVringAddr

	control *readerControl
}

func (vq *Virtq) MapRing(du, av, uu unsafe.Pointer) {
	vq.Vring.Desc = unsafe.Slice((*VringDesc)(du), vq.Vring.Num)

	vq.Vring.Used = (*VringUsed)(uu)
	vq.Vring.UsedRing = unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num)
	//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
	vq.Vring.UsedAvailEvent = (*uint16)(unsafe.Pointer(&unsafe.Slice(&vq.Vring.Used.Ring0, vq.Vring.Num+1)[vq.Vring.Num]))

	vq.Vring.Avail = (*VringAvail)(av)
	vq.Vring.AvailRing = unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num)
	//if (vu_has_feature(dev, VIRTIO_RING_F_EVENT_IDX)) {
	vq.Vring.AvailUsedEvent = &unsafe.Slice(&vq.Vring.Avail.Ring0, vq.Vring.Num+1)[vq.Vring.Num]
}

func (vq *Virtq) SetEnable(handle func(*VirtqElem) int) {
	var enable uint
	if handle != nil {
		enable = 1
	}

	vq.Enable = enable
	if vq.Enable != 0 {
		vq.control = &readerControl{
			cancel: make(chan struct{}, 1),
			done:   make(chan struct{}, 1),
		}
		go vq.readLoop(handle)
	} else if vq.control != nil {
		close(vq.control.cancel)
		<-vq.control.done
		vq.control = nil
	}
}

// popBatch drains the avail ring under the dispatch read lock and returns all
// available elements.  The lock is released before returning so that
// control-plane messages can be processed concurrently with request handling.
func (vq *Virtq) popBatch() []*VirtqElem {
	vq.dispatchMu.RLock()
	defer vq.dispatchMu.RUnlock()

	var batch []*VirtqElem
	for {
		data, err := vq.popQueue()
		if err != nil {
			log.Printf("popq: %v", err)
			break
		}
		if data == nil {
			break
		}
		batch = append(batch, data)
	}
	return batch
}

// popQueue removes one element from the available ring.
func (vq *Virtq) popQueue() (*VirtqElem, error) {
	vq.mu.Lock()
	defer vq.mu.Unlock()
	if !vq.Vring.Initialized() {
		return nil, fmt.Errorf("not initialized")
	}
	if vq.ResubmitList != nil && vq.ResubmitNum > 0 {
		return nil, fmt.Errorf("resubmit")
	}

	if vq.queueEmpty() {
		return nil, nil
	}

	if int(vq.inuse) >= vq.Vring.Num {
		return nil, fmt.Errorf("virtq size exceeded")
	}

	barrier.Read()

	idx := int(vq.LastAvailIdx) % vq.Vring.Num

	vq.LastAvailIdx++
	head := vq.Vring.AvailRing[idx]
	if int(head) >= vq.Vring.Num {
		return nil, fmt.Errorf("silly avail %d %d", head, vq.Vring.Num)
	}
	if vq.Vring.UsedAvailEvent != nil {
		*vq.Vring.UsedAvailEvent = vq.LastAvailIdx
	}

	// vu_queue_map_desc
	elem, err := vq.queueMapDesc(head)
	if elem == nil || err != nil {
		return nil, err
	}
	vq.inuse++
	vq.queueInflightGet(int(head))
	return elem, nil
}

// pushQueue writes one completed element into the used ring.
func (vq *Virtq) pushQueue(elem *VirtqElem, len int) {
	vq.mu.Lock()
	defer vq.mu.Unlock()
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
	barrier.Write()

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

/*
  Claude: This is the VHOST_USER_PROTOCOL_F_INFLIGHT_SHMFD feature, used for crash recovery across backend
  restarts.

  The inflight region is a shared memory segment (passed via fd from QEMU) that survives backend crashes. It tracks which descriptor
  chains were mid-flight at the time of a crash so they can be resubmitted after reconnection.

  VuVirtqInflight (the shared region):
  - desc[] -- one VuDescStateSplit per descriptor slot:
    - inflight = 1 when a descriptor was popped but not yet completed
    - counter -- a monotonically increasing sequence number recording the order descriptors were dequeued
  - used_idx -- snapshot of used->idx at last completion, used to detect a crash mid-completion
  - last_batch_head -- the descriptor being completed at crash time

  VuVirtqInflightDesc (the resubmit list, host-only):
  - Built at reconnect time from the shared region
  - Sorted by counter to restore original submission order
  - Drained first by vu_queue_pop before processing new requests

  The lifecycle of one request:

  1. vu_queue_inflight_get -- sets desc[i].inflight = 1, stamps counter
  2. vu_queue_inflight_pre_put -- records last_batch_head = desc_idx
  3. vu_queue_inflight_post_put -- clears desc[i].inflight = 0, then updates used_idx

  The barrier() calls between steps in post_put and the crash recovery code (line 1253-1258) handle a specific race: if the backend
  crashes between clearing inflight and updating used_idx, the recovery code detects the mismatch (inflight->used_idx != vq->used_idx)
  and uses last_batch_head to clear the descriptor that was half-completed.

  resubmit_list is the transient host-side list built at reconnect from all entries still marked inflight = 1 in shared memory, sorted
  by counter so resubmission happens in the original request order.
*/

func (vq *Virtq) queueInflightGet(head int) {
	// VHOST_USER_PROTOCOL_F_INFLIGHT_SHMFD
	if vq.Inflight == nil {
		// always returns here
		return
	}

	vq.InflightDescs[head].counter = vq.Counter
	vq.Counter++
	vq.InflightDescs[head].inflight = 1
}

// vringNotify checks whether a used-ring notification should be sent.
func (vq *Virtq) vringNotify() bool {
	vq.mu.Lock()
	defer vq.mu.Unlock()
	if !vq.Vring.Initialized() {
		return false
	}

	barrier.Full()

	// if F_NOTIFY_ON_EMPTY ...

	// if ! F_EVENT_IDX ...

	v := vq.SignaledUsedValid
	old := vq.SignaledUsed
	new := vq.UsedIdx
	vq.SignaledUsed = new
	vq.SignaledUsedValid = true
	return !v || VringNeedEvent(*vq.Vring.AvailUsedEvent, new, old)
}

// virtio-ring.h
func VringNeedEvent(eventIdx uint16, newIdx, old uint16) bool {
	return newIdx-eventIdx-1 < newIdx-old
}

func (vq *Virtq) queueNotify() {
	if !vq.vringNotify() {
		return
	}

	// if INBAND_NOTIFICATIONS ...
	var payload [8]byte
	payload[0] = 1
	if _, err := syscall.Write(vq.CallFD, payload[:]); err != nil {
		log.Panicf("eventfd write: %v", err)
	}
}

// queueMapDesc resolves descriptor chain starting at head into a VirtqElem.
func (vq *Virtq) queueMapDesc(head uint16) (*VirtqElem, error) {
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

		indirectAsBytes := vq.regions.FromGuestAddr(desc.Addr, uint64(desc.Len))
		if indirectAsBytes == nil {
			return nil, fmt.Errorf("OOB read %x", desc.Addr)
		}
		if len(indirectAsBytes) != int(desc.Len) {
			return nil, fmt.Errorf("partial read indirect desc")
		}
		n := desc.Len / uint32(eltSize)
		descArray = unsafe.Slice((*VringDesc)(unsafe.Pointer(&indirectAsBytes[0])), n)
		desc = descArray[0]
	}

	for {
		iov, err := vq.readVringEntry(desc.Addr, desc.Len)
		if err != nil {
			return nil, err
		}
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

		head = barrier.LoadUint16(&desc.Next)
		if head >= uint16(len(descArray)) {
			return nil, fmt.Errorf("OOB read, head %d beyond %d", head, len(descArray))
		}
		desc = descArray[head]
	}

	return &result, nil
}

// readVringEntry splits a guest physical address range into host byte slices,
// following region boundaries.
func (vq *Virtq) readVringEntry(physAddr uint64, sz uint32) ([][]byte, error) {
	var result [][]byte

	for sz > 0 {
		d := vq.regions.FromGuestAddr(physAddr, uint64(sz))
		if d == nil {
			return nil, fmt.Errorf("readVringEntry OOB: addr 0x%x (sz 0x%x)", physAddr, sz)
		}
		result = append(result, d)
		sz -= uint32(len(d))
		physAddr += uint64(len(d))
	}

	return result, nil
}

type readerControl struct {
	cancel chan struct{}
	done   chan struct{}
}

func (vq *Virtq) SetVringAddr(addr *VhostVringAddr) error {
	vq.Addr = *addr
	vq.Vring.Flags = uint32(addr.Flags)

	vq.Vring.LogGuestAddr = addr.LogGuestAddr

	du := vq.regions.FromDriverAddr(addr.DescUserAddr)
	av := vq.regions.FromDriverAddr(addr.AvailUserAddr)
	uu := vq.regions.FromDriverAddr(addr.UsedUserAddr)
	if du == nil {
		return fmt.Errorf("could not map DescUserAddr %x", addr.DescUserAddr)
	}
	if av == nil {
		return fmt.Errorf("could not map AvailUserAddr %x", addr.AvailUserAddr)
	}
	if uu == nil {
		return fmt.Errorf("could not map UsedUserAddr %x", addr.UsedUserAddr)
	}
	vq.MapRing(du, av, uu)

	vq.UsedIdx = vq.Vring.Used.Idx
	if vq.LastAvailIdx != vq.UsedIdx {
		// device->processed_in_order()
		vq.ShadowAvailIdx = vq.UsedIdx
		vq.LastAvailIdx = vq.UsedIdx
	}
	return nil
}

func newVirtq(dev *Device) *Virtq {
	return &Virtq{
		Notification: true,
		regions:      &dev.regions,
		dispatchMu:   &dev.dispatchMu,
		Debug:        &dev.Debug,
		KickFD:       -1,
		ErrFD:        -1,
		CallFD:       -1,
	}
}

func (vq *Virtq) Close() error {
	if vq.control != nil {
		close(vq.control.cancel)
		<-vq.control.done
		vq.control = nil
	}

	for _, fd := range []*int{&vq.KickFD, &vq.ErrFD, &vq.CallFD} {
		if *fd >= 0 {
			syscall.Close(*fd)
		}
		*fd = -1
	}

	return nil
}

func (vq *Virtq) fetchAvailIdx() {
	// read from shared memory.
	vq.ShadowAvailIdx = vq.Vring.Avail.Idx
}

func (vq *Virtq) queueEmpty() bool {
	// vq.vring == nil

	if vq.ShadowAvailIdx != vq.LastAvailIdx {
		return false
	}
	vq.fetchAvailIdx()
	return vq.ShadowAvailIdx == vq.LastAvailIdx
}
