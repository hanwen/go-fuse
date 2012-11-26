package fuse

import (
	"fmt"
	"log"
	"sync"
	"unsafe"
)

// HandleMap translates objects in Go space to 64-bit handles that can
// be given out to -say- the linux kernel.  It uses the free bits on
// x64_64 (16+3) to do an extra sanity check on the data.  (Thanks to
// Russ Cox for this suggestion).  In addition, it stores the object
// in a map, so the Go runtime will not garbage collect it.
//
// The 32 bits version of this is a threadsafe wrapper around a map.
//
// To use it, include Handled as first member of the structure
// you wish to export.
//
// This structure is thread-safe.
type HandleMap interface {
	Register(obj *Handled) uint64
	Count() int
	Decode(uint64) *Handled
	Forget(handle uint64, count int) (bool, *Handled)
	Handle(obj* Handled) uint64
	Has(uint64) bool
}

type Handled struct {
	check  uint32
	handle uint64
	count  int
}

func (h *Handled) verify() {
	if h.count < 0 {
		log.Panicf("negative lookup count %d", h.count)
	}
	if (h.count == 0) != (h.handle == 0) {
		log.Panicf("registration mismatch: lookup %d id %d", h.count, h.handle)
	}
}

const _ALREADY_MSG = "Object already has a handle"

////////////////////////////////////////////////////////////////
// portable version using 32 bit integers.

type portableHandleMap struct {
	sync.RWMutex
	used    int
	handles []*Handled
	freeIds []uint64
}

func newPortableHandleMap() *portableHandleMap {
	return &portableHandleMap{
		// Avoid handing out ID 0 and 1.
		handles: []*Handled{nil, nil},
	}
}

func (m *portableHandleMap) Register(obj *Handled) (handle uint64) {
	m.Lock()
	if obj.count == 0 {
		if obj.check != 0 {
			panic(_ALREADY_MSG)
		}

		if len(m.freeIds) == 0 {
			handle = uint64(len(m.handles))
			m.handles = append(m.handles, obj)
		} else {
			handle = m.freeIds[len(m.freeIds)-1]
			m.freeIds = m.freeIds[:len(m.freeIds)-1]
			m.handles[handle] = obj
		}
		m.used++
		obj.handle = handle
	} else {
		handle = obj.handle
	}
	obj.count++
	m.Unlock()
	return handle
}

func (m *portableHandleMap) Handle(obj *Handled) (h uint64) {
	m.RLock()
	if obj.count == 0 {
		h = 0
	} else {
		h = obj.handle
	}
	m.RUnlock()
	return h
}

func (m *portableHandleMap) Count() int {
	m.RLock()
	c := m.used
	m.RUnlock()
	return c
}

func (m *portableHandleMap) Decode(h uint64) *Handled {
	m.RLock()
	v := m.handles[h]
	m.RUnlock()
	return v
}

func (m *portableHandleMap) Forget(h uint64, count int) (forgotten bool, obj *Handled) {
	m.Lock()
	obj = m.handles[h]
	obj.count -= count
	if obj.count < 0 {
		panic("underflow")
	} else if obj.count == 0 {
		m.handles[h] = nil
		m.freeIds = append(m.freeIds, h)
		m.used--
		forgotten = true
		obj.handle = 0
	}
	m.Unlock()
	return forgotten, obj
}

func (m *portableHandleMap) Has(h uint64) bool {
	m.RLock()
	ok := m.handles[h] != nil
	m.RUnlock()
	return ok
}

// 32 bits version of HandleMap
type int32HandleMap struct {
	mutex   sync.Mutex
	handles map[uint32]*Handled
}

func (m *int32HandleMap) Register(obj *Handled) (handle uint64) {
	m.mutex.Lock()
	h := uint32(uintptr(unsafe.Pointer(obj)))
	if obj.count == 0 {
		m.handles[h] = obj
		obj.handle = uint64(h)
	}
	handle = uint64(h)
	obj.count++
	m.mutex.Unlock()
	return uint64(handle)
}

func (m *int32HandleMap) Has(h uint64) bool {
	m.mutex.Lock()
	ok := m.handles[uint32(h)] != nil
	m.mutex.Unlock()
	return ok
}

func (m *int32HandleMap) Handle(obj *Handled) uint64 {
	if obj.count == 0 {
		return 0
	}

	h := uint32(uintptr(unsafe.Pointer(obj)))
	return uint64(h)
}

func (m *int32HandleMap) Count() int {
	m.mutex.Lock()
	c := len(m.handles)
	m.mutex.Unlock()
	return c
}

func (m *int32HandleMap) Forget(handle uint64, count int) (forgotten bool, obj *Handled) {
	obj = m.Decode(handle)

	m.mutex.Lock()
	obj.count -= count
	if obj.count == 0 {
		obj.check = 0
		delete(m.handles, uint32(handle))
		forgotten = true
	} else if obj.count < 0 {
		panic("underflow")
	}
	obj.handle = 0
	m.mutex.Unlock()
	return forgotten, obj
}

func (m *int32HandleMap) Decode(handle uint64) *Handled {
	val := (*Handled)(unsafe.Pointer(uintptr(handle & ((1 << 32) - 1))))
	return val
}
func newInt32HandleMap() *int32HandleMap {
	return &int32HandleMap{
		handles: make(map[uint32]*Handled),
	}
}

// 64 bits version of HandleMap
type int64HandleMap struct {
	mutex    sync.Mutex
	handles  map[uint64]*Handled
	nextFree uint32
}

func (m *int64HandleMap) verify() {
	if !paranoia {
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range m.handles {
		if m.Decode(k) != v {
			panic("handle map out of sync")
		}
	}
}

func newInt64HandleMap() *int64HandleMap {
	return &int64HandleMap{
		handles:  make(map[uint64]*Handled),
		nextFree: 1, // to make tests easier.
	}
}

// NewHandleMap creates a new HandleMap.  If verify is given, we
// use remaining bits in the handle to store sanity check bits.
func NewHandleMap(portable bool) (hm HandleMap) {
	if portable {
		return newPortableHandleMap()
	}

	var obj *Handled
	switch unsafe.Sizeof(obj) {
	case 8:
		return newInt64HandleMap()
	case 4:
		return newInt32HandleMap()
	default:
		log.Fatalf("Unknown size.")
	}

	return nil
}

func (m *int64HandleMap) Count() int {
	m.mutex.Lock()
	c := len(m.handles)
	m.mutex.Unlock()
	return c
}

func (m *int64HandleMap) Register(obj *Handled) (handle uint64) {
	defer m.verify()

	m.mutex.Lock()
	if obj.count == 0 {
		handle = uint64(uintptr(unsafe.Pointer(obj)))

		rest := (handle &^ (1<<48 - 1))
		if rest != 0 {
			panic("more than 48 bits in address")
		}
		if handle&0x7 != 0 {
			panic("unaligned ptr")
		}
		handle >>= 3

		check := m.nextFree
		m.nextFree++
		m.nextFree = m.nextFree & (1<<(64-48+3) - 1)

		handle |= uint64(check) << (48 - 3)
		if obj.check != 0 {
			panic(_ALREADY_MSG)
		}
		obj.check = check
		obj.handle = handle
		m.handles[handle] = obj
	} else {
		handle = m.Handle(obj)
	}
	obj.count ++
	m.mutex.Unlock()

	return handle
}

func (m *int64HandleMap) Handle(obj *Handled) (handle uint64) {
	if obj.count == 0 {
		return 0
	}

	handle = uint64(uintptr(unsafe.Pointer(obj)))
	handle >>= 3
	handle |= uint64(obj.check) << (48 - 3)
	return handle
}


func (m *int64HandleMap) Forget(handle uint64, count int) (forgotten bool, obj *Handled) {
	defer m.verify()

	obj = m.Decode(handle)

	m.mutex.Lock()
	obj.count -= count
	if obj.count == 0 {
		delete(m.handles, handle)
		obj.check = 0
		obj.handle = 0
		forgotten = true
	} else if obj.count < 0 {
		panic("underflow")
	}
	m.mutex.Unlock()
	return forgotten, obj
}

func (m *int64HandleMap) Has(handle uint64) bool {
	m.mutex.Lock()
	ok := m.handles[handle] != nil
	m.mutex.Unlock()
	return ok
}

func (m *int64HandleMap) Decode(handle uint64) (val *Handled) {
	ptrBits := uintptr(handle & (1<<45 - 1))
	check := uint32(handle >> 45)
	val = (*Handled)(unsafe.Pointer(ptrBits << 3))
	if val.check != check {
		msg := fmt.Sprintf("handle check mismatch; handle has 0x%x, object has 0x%x",
			check, val.check)
		panic(msg)
	}
	return val
}
