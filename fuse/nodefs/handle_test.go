// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"os"
	"testing"
)

var skipTestHandleMapGeneration = false

func TestMain(m *testing.M) {
	// Test both handleMap implementations
	newHandleMap = newPortableHandleMap
	r := m.Run()
	if r != 0 {
		os.Exit(r)
	}
	newHandleMap = newSimpleHandleMap
	skipTestHandleMapGeneration = true
	r = m.Run()
	if r != 0 {
		os.Exit(r)
	}
}

func TestHandleMapLookupCount(t *testing.T) {
	for _, portable := range []bool{true, false} {
		t.Log("portable:", portable)
		v := new(handled)
		hm := newHandleMap()
		h1, g1 := hm.Register(v)
		h2, g2 := hm.Register(v)

		if h1 != h2 {
			t.Fatalf("double register should reuse handle: got %d want %d.", h2, h1)
		}

		if g1 != g2 {
			t.Fatalf("double register should reuse generation: got %d want %d.", g2, g1)
		}

		hm.Register(v)

		forgotten, obj := hm.Forget(h1, 1)
		if forgotten {
			t.Fatalf("single forget unref forget object.")
		}

		if obj != v {
			t.Fatalf("should return input object.")
		}

		if !hm.Has(h1) {
			t.Fatalf("handlemap.Has() returned false for live object.")
		}

		forgotten, obj = hm.Forget(h1, 2)
		if !forgotten {
			t.Fatalf("unref did not forget object.")
		}

		if obj != v {
			t.Fatalf("should return input object.")
		}

		if hm.Has(h1) {
			t.Fatalf("handlemap.Has() returned false for live object.")
		}
	}
}

func TestHandleMapBasic(t *testing.T) {
	v := new(handled)
	hm := newHandleMap()
	h, _ := hm.Register(v)
	t.Logf("Got handle 0x%x", h)
	if !hm.Has(h) {
		t.Fatal("Does not have handle")
	}
	if hm.Handle(v) != h {
		t.Fatalf("handle mismatch, got %x want %x", hm.Handle(v), h)
	}
	if hm.Decode(h) != v {
		t.Fatal("address mismatch")
	}
	if hm.Count() != 1 {
		t.Fatal("count error")
	}
	hm.Forget(h, 1)
	if hm.Count() != 0 {
		t.Fatal("count error")
	}
	if hm.Has(h) {
		t.Fatal("Still has handle")
	}
}

func TestHandleMapMultiple(t *testing.T) {
	hm := newHandleMap()
	for i := 0; i < 10; i++ {
		v := &handled{}
		h, _ := hm.Register(v)
		if hm.Decode(h) != v {
			t.Fatal("address mismatch")
		}
		if hm.Count() != i+1 {
			t.Fatal("count error")
		}
	}
}

func TestHandleMapGeneration(t *testing.T) {
	if skipTestHandleMapGeneration {
		t.Skip("simpleHandleMap never reuses handles")
	}

	hm := newHandleMap()

	h1, g1 := hm.Register(&handled{})

	forgotten, _ := hm.Forget(h1, 1)
	if !forgotten {
		t.Fatalf("unref did not forget object.")
	}

	h2, g2 := hm.Register(&handled{})

	if h1 != h2 {
		t.Fatalf("register should reuse handle: got %d want %d.", h2, h1)
	}

	if g1 >= g2 {
		t.Fatalf("register should increase generation: got %d want greater than %d.", g2, g1)
	}
}
