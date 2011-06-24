package fuse
import (
	"fmt"
	"unsafe"
	"sync"
)

// HandleMap translates objects in Go space to 64-bit handles that can
// be given out to -say- the linux kernel.  It uses the free bits
// (16+3 on x64_64 and 32 on x86) to do an extra sanity check on the
// data.  (Thanks to Russ Cox for this suggestion).  In addition, it
// stores the object in a map, so the Go runtime will not garbage
// collect it.
//
// To use it, include Handled as first member of the structure
// you wish to export.
//
// This structure is thread-safe.
type HandleMap struct {
	mutex sync.Mutex
	handles map[uint64]*Handled
	nextFree uint32 
}

func (me *HandleMap) verify() {
	if !paranoia {
		return
	}
	
	me.mutex.Lock()
	defer me.mutex.Unlock()
	for k, v := range me.handles {
		if DecodeHandle(k) != v {
			panic("handle map out of sync")
		}
	}
}

func NewHandleMap() *HandleMap {
	return &HandleMap{
		handles:  make(map[uint64]*Handled),
		nextFree: 1,	// to make tests easier.
	}
}

type Handled struct {
	check uint32
}

func (me *HandleMap) Count() int {
	me.mutex.Lock()
	defer me.mutex.Unlock()
	return len(me.handles)
}

func (me *HandleMap) Register(obj *Handled) (handle uint64) {
	if obj.check != 0 {
		panic("Object already has a handle.")
	}
	me.mutex.Lock()
	defer me.mutex.Unlock()
	
	handle = uint64(uintptr(unsafe.Pointer(obj)))
	check := me.nextFree
	me.nextFree++
	if unsafe.Sizeof(obj) == 8 {
		me.nextFree = me.nextFree & (1 << (64 - 48 + 3) -1)
		
		rest := (handle &^ (1<<48 - 1)) | (handle & (1<<3 -1))
		if rest != 0 {
			panic("unaligned ptr or more than 48 bits in address")
		}
		handle >>= 3
		handle |= uint64(obj.check) <<  (64 - 48 + 3)
	}
	
	if unsafe.Sizeof(obj) == 4 {
		rest := (handle & 0x3)
		if rest != 0 {
			panic("unaligned ptr")
		}
		
		handle |= uint64(check) << 32
	}
	obj.check = check
	me.handles[handle] = obj
	return handle
}

func (me *HandleMap) Forget(handle uint64) (val *Handled) {
	val = DecodeHandle(handle)

	me.mutex.Lock()
	defer me.mutex.Unlock()
	val.check = 0
	me.handles[handle] = nil, false
	return val
}

func DecodeHandle(handle uint64) (val *Handled) {
	var check uint32
	if unsafe.Sizeof(val) == 8 {
		ptrBits := uintptr(handle & (1<<45-1))
		check = uint32(handle >> 45)
		val = (*Handled)(unsafe.Pointer(ptrBits<<3))
	}
	if unsafe.Sizeof(val) == 4 {
		check = uint32(handle >> 32)
		val = (*Handled)(unsafe.Pointer(uintptr(handle & ((1<<32)-1))))
	}
	if val.check != check {
		msg := fmt.Sprintf("handle check mismatch; handle has 0x%x, object has 0x%x",
			check, val.check)
 		panic(msg)
	}
	return val
}

