package fuse

import (
	"log"
	"strings"
	"testing"
	"unsafe"
)

func markSeen(substr string) {
	if r := recover(); r != nil {
		s := r.(string)
		if strings.Contains(s, substr) {
			log.Println("expected recovery from: ", r)
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
	log.Println("TestDoubleRegister")
	defer markSeen("already has a handle")
	hm := NewHandleMap()
	hm.Register(&Handled{})
	v := &Handled{}
	hm.Register(v)
	hm.Register(v)
	t.Error("Double register did not panic")
}

func TestHandleMapUnaligned(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	hm := NewHandleMap()

	b := make([]byte, 100)
	v := (*Handled)(unsafe.Pointer(&b[1]))

	defer markSeen("unaligned")
	hm.Register(v)
	t.Error("Unaligned register did not panic")
}

func TestHandleMapPointerLayout(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}

	hm := NewHandleMap()
	bogus := uint64(1) << uint32((8 * (unsafe.Sizeof(t) - 1)))
	p := uintptr(bogus)
	v := (*Handled)(unsafe.Pointer(p))
	defer markSeen("48")
	hm.Register(v)
	t.Error("bogus register did not panic")
}

func TestHandleMapBasic(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	v := new(Handled)
	hm := NewHandleMap()
	h := hm.Register(v)
	log.Printf("Got handle 0x%x", h)
	if DecodeHandle(h) != v {
		t.Fatal("address mismatch")
	}
	if hm.Count() != 1 {
		t.Fatal("count error")
	}
	hm.Forget(h)
	if hm.Count() != 0 {
		t.Fatal("count error")
	}
}

func TestHandleMapMultiple(t *testing.T) {
	if unsafe.Sizeof(t) < 8 {
		t.Log("skipping test for 32 bits")
		return
	}
	hm := NewHandleMap()
	for i := 0; i < 10; i++ {
		v := &Handled{}
		h := hm.Register(v)
		if DecodeHandle(h) != v {
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
	defer markSeen("check mismatch")

	v := new(Handled)
	hm := NewHandleMap()
	h := hm.Register(v)
	DecodeHandle(h | (uint64(1) << 63))
	t.Error("Borked decode did not panic")
}
