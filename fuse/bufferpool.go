package fuse

import (
	"sync"
	"fmt"
)

// This implements a pool of buffers that returns slices with capacity
// (2^e * PAGESIZE) for e=0,1,...  which have possibly been used, and
// may contain random contents.
type BufferPool struct {
	lock sync.Mutex

	// For each exponent a list of slice pointers.
	buffersByExponent [][][]byte
}

// Returns the smallest E such that 2^E >= Z.
func IntToExponent(z int) uint {
	x := z
	var exp uint = 0
	for x > 1 {
		exp++
		x >>= 1
	}

	if z > (1 << exp) {
		exp ++
	}
	return exp
}

func NewBufferPool() *BufferPool {
	bp := new(BufferPool)
	bp.buffersByExponent = make([][][]byte, 0, 8)
	return bp
}

func (self *BufferPool) String() string {
	s := ""
	for exp, bufs := range(self.buffersByExponent) {
		s = s + fmt.Sprintf("%d = %d\n", exp, len(bufs))
	}
	return s
}


func (self *BufferPool) getBuffer(sz int) []byte {
	exponent := int(IntToExponent(sz) - IntToExponent(PAGESIZE))
	self.lock.Lock()
	defer self.lock.Unlock()

	if (len(self.buffersByExponent) <= int(exponent)) {
		return nil
	}
	bufferList := self.buffersByExponent[exponent]
	if (len(bufferList) == 0) {
		return nil
	}

	result := bufferList[len(bufferList)-1]
	self.buffersByExponent[exponent] = self.buffersByExponent[exponent][:len(bufferList)-1]

	if cap(result) < sz {
		panic("returning incorrect buffer.")
	}

	return result
}

func (self *BufferPool) addBuffer(slice []byte) {
	if cap(slice) & (PAGESIZE -1) != 0 {
		return
	}

	pages := cap(slice) / PAGESIZE
	if pages == 0 {
		return
	}
	exp := IntToExponent(pages)
	if (1 << exp) != pages {
		return
	}

	self.lock.Lock()
	defer self.lock.Unlock()
	for len(self.buffersByExponent) <= int(exp) {
		self.buffersByExponent = append(self.buffersByExponent, make([][]byte, 0))
	}
	self.buffersByExponent[exp] = append(self.buffersByExponent[exp], slice)
}


func (self *BufferPool) GetBuffer(size uint32) []byte {
	sz := int(size)
	if sz < PAGESIZE {
		sz = PAGESIZE
	}
	rounded := 1 << IntToExponent(sz)
	b := self.getBuffer(rounded)

	if b != nil {
		b = b[:size]
		return b
	}

	return make([]byte, size, rounded)
}

