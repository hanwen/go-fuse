package fuse

import (
	"testing"
	"fmt"
)

func TestIntToExponent(t *testing.T) {
	e := IntToExponent(1)
	if e != 0 {
		t.Error("1", e)
	}
	e = IntToExponent(2)
	if e != 1 {
		t.Error("2", e)
	}
	e = IntToExponent(3)
	if e != 2 {
		t.Error("3", e)
	}
	e = IntToExponent(4)
	if e != 2 {
		t.Error("4", e)
	}
}

func TestBufferPool(t *testing.T) {
	bp := NewBufferPool()

	b := bp.getBuffer(PAGESIZE - 1)
	if b != nil {
		t.Error("bp 0")
	}
	b = bp.getBuffer(PAGESIZE)
	if b != nil {
		t.Error("bp 1")
	}

	s := make([]byte, PAGESIZE-1)

	bp.addBuffer(s)
	b = bp.getBuffer(PAGESIZE - 1)
	if b != nil {
		t.Error("bp 3")
	}

	s = make([]byte, PAGESIZE)
	bp.addBuffer(s)
	b = bp.getBuffer(PAGESIZE)
	if b == nil {
		t.Error("not found.")
	}

	b = bp.getBuffer(PAGESIZE)
	if b != nil {
		t.Error("should fail.")
	}

	bp.addBuffer(make([]byte, 3*PAGESIZE))
	b = bp.getBuffer(2 * PAGESIZE)
	if b != nil {
		t.Error("should fail.")
	}
	b = bp.getBuffer(4 * PAGESIZE)
	if b != nil {
		t.Error("should fail.")
	}
	bp.addBuffer(make([]byte, 4*PAGESIZE))
	fmt.Println(bp)
	b = bp.getBuffer(2 * PAGESIZE)
	if b != nil {
		t.Error("should fail.")
	}
	b = bp.getBuffer(4 * PAGESIZE)
	if b == nil {
		t.Error("4*ps should succeed.")
	}

}
