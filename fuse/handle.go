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
	Register(obj *Handled, asInt interface{}) uint64
	Count() int
	Decode(uint64) *Handled
	Forget(uint64) *Handled
	Has(uint64) bool
}

type Handled struct {
	check  uint32
	object interface{}
}

const _ALREADY_MSG = "Object already has a handle"
////////////////////////////////////////////////////////////////
// portable version using 32 bit integers.

type portableHandleMap struct {
	sync.RWMutex
	nextFree uint32
	handles  map[uint64]*Handled
}

func (me *portableHandleMap) Register(obj *Handled, asInt interface{}) uint64 {
	if obj.check != 0 {
		panic(_ALREADY_MSG)
	}
	me.Lock()
	defer me.Unlock()
	for {
		h := uint64(me.nextFree)
		me.nextFree++
		// HACK - we make sure we start with 1, so we always
		// assign root to 1.
		if h < 1 {
			continue
		}
		old := me.handles[h]
		if old != nil {
			continue
		}

		me.handles[h] = obj
		obj.check = 0xbaabbaab
		return h
	}

	return 0
}

func (me *portableHandleMap) Count() int {
	me.RLock()
	defer me.RUnlock()
	return len(me.handles)
}

func (me *portableHandleMap) Decode(h uint64) *Handled {
	me.RLock()
	defer me.RUnlock()
	return me.handles[h]
}

func (me *portableHandleMap) Forget(h uint64) *Handled {
	me.Lock()
	defer me.Unlock()
	v := me.handles[h]
	v.check = 0
	me.handles[h] = nil, false
	return v
}

func (me *portableHandleMap) Has(h uint64) bool {
	me.RLock()
	defer me.RUnlock()
	return me.handles[h] != nil
}

// 32 bits version of HandleMap
type int32HandleMap struct {
	mutex   sync.Mutex
	handles map[uint32]*Handled
}

func (me *int32HandleMap) Register(obj *Handled, asInt interface{}) uint64 {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	handle := uint32(uintptr(unsafe.Pointer(obj)))
	me.handles[handle] = obj
	return uint64(handle)
}

func (me *int32HandleMap) Has(h uint64) bool {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	return me.handles[uint32(h)] != nil
}

func (me *int32HandleMap) Count() int {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	return len(me.handles)
}

func (me *int32HandleMap) Forget(handle uint64) *Handled {
	val := me.Decode(handle)

	me.mutex.Lock()
	defer me.mutex.Unlock()
	val.check = 0
	me.handles[uint32(handle)] = nil, false
	return val
}

func (me *int32HandleMap) Decode(handle uint64) *Handled {
	val := (*Handled)(unsafe.Pointer(uintptr(handle & ((1 << 32) - 1))))
	return val
}

// 64 bits version of HandleMap
type int64HandleMap struct {
	mutex    sync.Mutex
	handles  map[uint64]*Handled
	nextFree uint32
}

var baseAddress uint64

func (me *int64HandleMap) verify() {
	if !paranoia {
		return
	}

	me.mutex.Lock()
	defer me.mutex.Unlock()
	for k, v := range me.handles {
		if me.Decode(k) != v {
			panic("handle map out of sync")
		}
	}
}

// NewHandleMap creates a new HandleMap.  If verify is given, we
// use remaining bits in the handle to store sanity check bits.
func NewHandleMap(portable bool) (hm HandleMap) {
	if portable {
		return &portableHandleMap{
			handles: make(map[uint64]*Handled),
		}
	}

	var obj *Handled
	switch unsafe.Sizeof(obj) {
	case 8:
		return &int64HandleMap{
			handles:  make(map[uint64]*Handled),
			nextFree: 1, // to make tests easier.
		}
	case 4:
		return &int32HandleMap{
			handles: make(map[uint32]*Handled),
		}
	default:
		log.Println("Unknown size.")
	}

	return nil
}

func (me *int64HandleMap) Count() int {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	return len(me.handles)
}

func (me *int64HandleMap) Register(obj *Handled, asInterface interface{}) (handle uint64) {
	defer me.verify()

	me.mutex.Lock()
	defer me.mutex.Unlock()

	handle = uint64(uintptr(unsafe.Pointer(obj)))

	rest := (handle &^ (1<<48 - 1))
	if rest != 0 {
		panic("more than 48 bits in address")
	}
	if handle&0x7 != 0 {
		panic("unaligned ptr")
	}
	handle -= baseAddress
	handle >>= 3

	check := me.nextFree
	me.nextFree++
	me.nextFree = me.nextFree & (1<<(64-48+3) - 1)

	handle |= uint64(check) << (48 - 3)
	if obj.check != 0 {
		panic(_ALREADY_MSG)
	}
	obj.check = check

	obj.object = asInterface
	me.handles[handle] = obj
	return handle
}

func (me *int64HandleMap) Forget(handle uint64) (val *Handled) {
	defer me.verify()

	val = me.Decode(handle)

	me.mutex.Lock()
	defer me.mutex.Unlock()
	me.handles[handle] = nil, false
	val.check = 0
	return val
}

func (me *int64HandleMap) Has(handle uint64) bool {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	return me.handles[handle] != nil
}

func (me *int64HandleMap) Decode(handle uint64) (val *Handled) {
	ptrBits := uintptr(handle & (1<<45 - 1))
	check := uint32(handle >> 45)
	val = (*Handled)(unsafe.Pointer(ptrBits<<3 + uintptr(baseAddress)))

	if val.check != check {
		msg := fmt.Sprintf("handle check mismatch; handle has 0x%x, object has 0x%x: %v",
			check, val.check, val.object)
		panic(msg)
	}
	return val
}

func init() {
	// TODO - figure out a way to discover this nicely.  This is
	// depends in a pretty fragile way on the 6g allocator
	// characteristics.
	baseAddress = uint64(0xf800000000)
}
