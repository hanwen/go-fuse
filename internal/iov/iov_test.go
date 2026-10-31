// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iov

import (
	"bytes"
	"testing"
)

func TestHasPrefixAt(t *testing.T) {
	v := [][]byte{
		[]byte("0123"),
		[]byte("45com."),
		[]byte("apple."),
		[]byte("provenance"),
	}
	if !HasPrefixAt(v, 6, []byte("com.apple.provenance")) {
		t.Errorf("HasPrefixAt across chunks = false, want true")
	}
	if HasPrefixAt(v, 6, []byte("com.apple.quarantine")) {
		t.Errorf("HasPrefixAt mismatch = true, want false")
	}
	if HasPrefixAt(v, 100, []byte("a")) {
		t.Errorf("HasPrefixAt past end = true, want false")
	}
}

func TestCopyAndSlice(t *testing.T) {
	v := [][]byte{[]byte("hello "), []byte("world")}
	if got := Len(v); got != 11 {
		t.Fatalf("Len = %d, want 11", got)
	}
	if s, ok := Slice(v, 0, 5); !ok || string(s) != "hello" {
		t.Errorf("Slice(0, 5) = %q, %v", s, ok)
	}
	if _, ok := Slice(v, 3, 5); ok {
		t.Errorf("Slice across boundary succeeded")
	}

	var dst [5]byte
	if n := CopyFrom(dst[:], v, 3); n != 5 || string(dst[:]) != "lo wo" {
		t.Errorf("CopyFrom = %d, %q", n, dst[:])
	}

	sub := Subslice(v, 3, 5)
	var flat []byte
	for _, b := range sub {
		flat = append(flat, b...)
	}
	if !bytes.Equal(flat, []byte("lo wo")) {
		t.Errorf("Subslice = %q, want %q", flat, "lo wo")
	}
}
