// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"testing"
)

// verify that flagString always formats flags in the same order.
func TestFlagStringOrder(t *testing.T) {
	var flags int64 = CAP_ASYNC_READ | CAP_SPLICE_WRITE | CAP_READDIRPLUS | CAP_MAX_PAGES | CAP_EXPLICIT_INVAL_DATA
	want := "ASYNC_READ,SPLICE_WRITE,READDIRPLUS,MAX_PAGES,EXPLICIT_INVAL_DATA"
	// loop many times to check for sure the order is untied from map iteration order
	for i := 0; i < 100; i++ {
		have := flagString(initFlagNames, flags, "")
		if have != want {
			t.Fatalf("flagString:\nhave: %q\nwant: %q", have, want)
		}
	}
}

// verify how flagString handles provided default.
func TestFlagStringDefault(t *testing.T) {
	names := newFlagNames(map[int64]string{
		1: "AAA",
		2: "BBB",
		4: "CCC",
	})

	testv := []struct {
		flags int64
		def   string
		strok string
	}{
		{0, "", ""},
		{0, "X", "X"},
		{1, "X", "AAA"},
		{5, "X", "AAA,CCC"},
		{8, "X", "0x8"},
		{9, "X", "AAA,0x8"},
	}

	for _, test := range testv {
		str := flagString(names, test.flags, test.def)
		if str != test.strok {
			t.Errorf("flagString(%x, %q) -> got %q ;  want %q",
				test.flags, test.def, str, test.strok)
		}
	}
}
