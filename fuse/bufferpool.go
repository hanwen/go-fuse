package fuse

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"unsafe"
)

var _ = log.Println

type BufferPool interface {
	AllocBuffer(size uint32) []byte
	FreeBuffer(slice []byte)
	String() string
}

type GcBufferPool struct {

}

// NewGcBufferPool is just a fallback to the standard allocation routines.
func NewGcBufferPool() *GcBufferPool {
	return &GcBufferPool{}
}

func (me *GcBufferPool) AllocBuffer(size uint32) []byte {
	return make([]byte, size)
}

func (me *GcBufferPool) FreeBuffer(slice []byte) {
}

// BufferPool implements a pool of buffers that returns slices with
// capacity of a multiple of PAGESIZE, which have possibly been used,
// and may contain random contents.
type BufferPoolImpl struct {
	lock sync.Mutex

	// For each page size multiple a list of slice pointers.
	buffersBySize [][][]byte

	// start of slice => true
	outstandingBuffers map[uintptr]bool

	// Total count of created buffers.  Handy for finding memory
	// leaks.
	createdBuffers int
}

func NewBufferPool() *BufferPoolImpl {
	bp := new(BufferPoolImpl)
	bp.buffersBySize = make([][][]byte, 0, 32)
	bp.outstandingBuffers = make(map[uintptr]bool)
	return bp
}

func (me *BufferPoolImpl) String() string {
	me.lock.Lock()
	defer me.lock.Unlock()

	result := []string{}
	for exp, bufs := range me.buffersBySize {
		if len(bufs) > 0 {
			result = append(result, fmt.Sprintf("%d=%d\n", exp, len(bufs)))
		}
	}
	return fmt.Sprintf("created: %v\noutstanding %v\n%s",
		me.createdBuffers, len(me.outstandingBuffers),
		strings.Join(result, ", "))
}

func (me *BufferPoolImpl) getBuffer(pageCount int) []byte {
	for ; pageCount < len(me.buffersBySize); pageCount++ {
		bufferList := me.buffersBySize[pageCount]
		if len(bufferList) > 0 {
			result := bufferList[len(bufferList)-1]
			me.buffersBySize[pageCount] = me.buffersBySize[pageCount][:len(bufferList)-1]
			return result
		}
	}

	return nil
}

func (me *BufferPoolImpl) addBuffer(slice []byte, pages int) {
	for len(me.buffersBySize) <= int(pages) {
		me.buffersBySize = append(me.buffersBySize, make([][]byte, 0))
	}
	me.buffersBySize[pages] = append(me.buffersBySize[pages], slice)
}

// AllocBuffer creates a buffer of at least the given size. After use,
// it should be deallocated with FreeBuffer().
func (me *BufferPoolImpl) AllocBuffer(size uint32) []byte {
	sz := int(size)
	if sz < PAGESIZE {
		sz = PAGESIZE
	}

	if sz%PAGESIZE != 0 {
		sz += PAGESIZE
	}
	psz := sz / PAGESIZE

	me.lock.Lock()
	defer me.lock.Unlock()

	var b []byte

	b = me.getBuffer(psz)
	if b == nil {
		me.createdBuffers++
		b = make([]byte, size, psz*PAGESIZE)
	} else {
		b = b[:size]
	}

	me.outstandingBuffers[uintptr(unsafe.Pointer(&b[0]))] = true

	// For testing should not have more than 20 buffers outstanding.
	if paranoia && (me.createdBuffers > 50 || len(me.outstandingBuffers) > 50) {
		panic("Leaking buffers")
	}

	return b
}

// FreeBuffer takes back a buffer if it was allocated through
// AllocBuffer.  It is not an error to call FreeBuffer() on a slice
// obtained elsewhere.
func (me *BufferPoolImpl) FreeBuffer(slice []byte) {
	if slice == nil {
		return
	}
	if cap(slice)%PAGESIZE != 0 || cap(slice) == 0 {
		return
	}
	psz := cap(slice) / PAGESIZE
	slice = slice[:psz]
	key := uintptr(unsafe.Pointer(&slice[0]))

	me.lock.Lock()
	defer me.lock.Unlock()
	ok := me.outstandingBuffers[key]
	if ok {
		me.addBuffer(slice, psz)
		delete(me.outstandingBuffers, key)
	}
}
