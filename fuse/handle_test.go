package fuse

import (
	"log"
	"strings"
	"testing"
	"unsafe"
)

var _ = log.Println

func markSeen(t *testing.T, substr string) {
	if r := recover(); r != nil {
		s := r.(string)
		if strings.Contains(s, substr) {
			t.Log("expected recovery from: ", r)
		} else {
			panic(s)
		}
	}
}

func TestHandleMapDoubleRegister(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	t.Log("TestDoubleRegister")
	defer markSeen(t, "already has a handle")
	hm := NewHandleMap(false)
	obj := &Handled{}
	hm.Register(obj, obj)
	v := &Handled{}
	hm.Register(v, v)
	hm.Register(v, v)
	t.Error("Double register did not panic")
}

func TestHandleMapUnaligned(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	hm := NewHandleMap(false)

	b := make([]byte, 100)
	v := (*Handled)(unsafe.Pointer(&b[1]))

	defer markSeen(t, "unaligned")
	hm.Register(v, v)
	t.Error("Unaligned register did not panic")
}

func TestHandleMapPointerLayout(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}

	hm := NewHandleMap(false)
	bogus := uint64(1) << uint32((8 * (unsafe.Sizeof(t) - 1)))
	p := uintptr(bogus)
	v := (*Handled)(unsafe.Pointer(p))
	defer markSeen(t, "48")
	hm.Register(v, v)
	t.Error("bogus register did not panic")
}

func TestHandleMapBasic(t *testing.T) {
	for _, portable := range []bool{true, false} {
		v := new(Handled)
		hm := NewHandleMap(portable)
		h := hm.Register(v, v)
		t.Logf("Got handle 0x%x", h)
		if !hm.Has(h) {
			t.Fatal("Does not have handle")
		}
		if hm.Decode(h) != v {
			t.Fatal("address mismatch")
		}
		if hm.Count() != 1 {
			t.Fatal("count error")
		}
		hm.Forget(h)
		if hm.Count() != 0 {
			t.Fatal("count error")
		}
		if hm.Has(h) {
			t.Fatal("Still has handle")
		}
		if v.check != 0 {
			t.Errorf("forgotten object still has a check.")
		}
	}
}

func TestHandleMapMultiple(t *testing.T) {
	hm := NewHandleMap(false)
	for i := 0; i < 10; i++ {
		v := &Handled{}
		h := hm.Register(v, v)
		if hm.Decode(h) != v {
			t.Fatal("address mismatch")
		}
		if hm.Count() != i+1 {
			t.Fatal("count error")
		}
	}
}

func TestHandleMapCheckFail(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	defer markSeen(t, "check mismatch")

	v := new(Handled)
	hm := NewHandleMap(false)
	h := hm.Register(v, v)
	hm.Decode(h | (uint64(1) << 63))
	t.Error("Borked decode did not panic")
}
