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

func (p *GcBufferPool) AllocBuffer(size uint32) []byte {
	return make([]byte, size)
}

func (p *GcBufferPool) FreeBuffer(slice []byte) {
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

func (p *BufferPoolImpl) String() string {
	p.lock.Lock()
	defer p.lock.Unlock()

	result := []string{}
	for exp, bufs := range p.buffersBySize {
		if len(bufs) > 0 {
			result = append(result, fmt.Sprintf("%d=%d", exp, len(bufs)))
		}
	}
	return fmt.Sprintf("created: %d, outstanding %d. Sizes: %s",
		p.createdBuffers, len(p.outstandingBuffers),
		strings.Join(result, ", "))
}

func (p *BufferPoolImpl) getBuffer(pageCount int) []byte {
	for ; pageCount < len(p.buffersBySize); pageCount++ {
		bufferList := p.buffersBySize[pageCount]
		if len(bufferList) > 0 {
			result := bufferList[len(bufferList)-1]
			p.buffersBySize[pageCount] = p.buffersBySize[pageCount][:len(bufferList)-1]
			return result
		}
	}

	return nil
}

func (p *BufferPoolImpl) addBuffer(slice []byte, pages int) {
	for len(p.buffersBySize) <= int(pages) {
		p.buffersBySize = append(p.buffersBySize, make([][]byte, 0))
	}
	p.buffersBySize[pages] = append(p.buffersBySize[pages], slice)
}

// AllocBuffer creates a buffer of at least the given size. After use,
// it should be deallocated with FreeBuffer().
func (p *BufferPoolImpl) AllocBuffer(size uint32) []byte {
	sz := int(size)
	if sz < PAGESIZE {
		sz = PAGESIZE
	}

	if sz%PAGESIZE != 0 {
		sz += PAGESIZE
	}
	psz := sz / PAGESIZE

	p.lock.Lock()
	var b []byte

	b = p.getBuffer(psz)
	if b == nil {
		p.createdBuffers++
		b = make([]byte, size, psz*PAGESIZE)
	} else {
		b = b[:size]
	}

	p.outstandingBuffers[uintptr(unsafe.Pointer(&b[0]))] = true

	// For testing should not have more than 20 buffers outstanding.
	if paranoia && (p.createdBuffers > 50 || len(p.outstandingBuffers) > 50) {
		panic("Leaking buffers")
	}
	p.lock.Unlock()

	return b
}

// FreeBuffer takes back a buffer if it was allocated through
// AllocBuffer.  It is not an error to call FreeBuffer() on a slice
// obtained elsewhere.
func (p *BufferPoolImpl) FreeBuffer(slice []byte) {
	if slice == nil {
		return
	}
	if cap(slice)%PAGESIZE != 0 || cap(slice) == 0 {
		return
	}
	psz := cap(slice) / PAGESIZE
	slice = slice[:psz]
	key := uintptr(unsafe.Pointer(&slice[0]))

	p.lock.Lock()
	ok := p.outstandingBuffers[key]
	if ok {
		p.addBuffer(slice, psz)
		delete(p.outstandingBuffers, key)
	}
	p.lock.Unlock()
}
