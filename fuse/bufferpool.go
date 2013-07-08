package fuse

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

var paranoia bool

// BufferPool implements explicit memory management. It is used for
// minimizing the GC overhead of communicating with the kernel.
type BufferPool interface {
	// AllocBuffer creates a buffer of at least the given size. After use,
	// it should be deallocated with FreeBuffer().
	AllocBuffer(size uint32) []byte

	// FreeBuffer takes back a buffer if it was allocated through
	// AllocBuffer.  It is not an error to call FreeBuffer() on a slice
	// obtained elsewhere.
	FreeBuffer(slice []byte)

	// Return debug information.
	String() string
}

type gcBufferPool struct {
}

// NewGcBufferPool is a fallback to the standard allocation routines.
func NewGcBufferPool() BufferPool {
	return &gcBufferPool{}
}

func (p *gcBufferPool) String() string {
	return "gc"
}

func (p *gcBufferPool) AllocBuffer(size uint32) []byte {
	return make([]byte, size)
}

func (p *gcBufferPool) FreeBuffer(slice []byte) {
}

type bufferPoolImpl struct {
	lock sync.Mutex

	// For each page size multiple a list of slice pointers.
	buffersBySize [][][]byte

	// start of slice => true
	outstandingBuffers map[uintptr]bool

	// Total count of created buffers.  Handy for finding memory
	// leaks.
	createdBuffers int
}

// NewBufferPool returns a BufferPool implementation that that returns
// slices with capacity of a multiple of PAGESIZE, which have possibly
// been used, and may contain random contents. When using
// NewBufferPool, file system handlers may not hang on to passed-in
// buffers beyond the handler's return.
func NewBufferPool() BufferPool {
	bp := new(bufferPoolImpl)
	bp.buffersBySize = make([][][]byte, 0, 32)
	bp.outstandingBuffers = make(map[uintptr]bool)
	return bp
}

func (p *bufferPoolImpl) String() string {
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

func (p *bufferPoolImpl) getBuffer(pageCount int) []byte {
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

func (p *bufferPoolImpl) addBuffer(slice []byte, pages int) {
	for len(p.buffersBySize) <= int(pages) {
		p.buffersBySize = append(p.buffersBySize, make([][]byte, 0))
	}
	p.buffersBySize[pages] = append(p.buffersBySize[pages], slice)
}

func (p *bufferPoolImpl) AllocBuffer(size uint32) []byte {
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

func (p *bufferPoolImpl) FreeBuffer(slice []byte) {
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
