package fuse

import (
	"testing"
)

func TestBufferPool(t *testing.T) {
	bp := NewBufferPool()

	b1 := bp.AllocBuffer(PAGESIZE)
	_ = bp.AllocBuffer(2 * PAGESIZE)
	bp.FreeBuffer(b1)

	b1_2 := bp.AllocBuffer(PAGESIZE)
	if &b1[0] != &b1_2[0] {
		t.Error("bp 0")
	}

}

func TestFreeBufferEmpty(t *testing.T) {
	bp := NewBufferPool()
	c := make([]byte, 0, 2*PAGESIZE)
	bp.FreeBuffer(c)
}
