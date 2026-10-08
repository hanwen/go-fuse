// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "testing"

func expectPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic")
		}
	}()
	f()
}

func TestDirEntryListNoMixing(t *testing.T) {
	buf := make([]byte, 1024)
	e := DirEntry{Name: "a", Ino: 1}

	l := NewDirEntryList(buf, 0)
	if !l.AddDirEntry(e) {
		t.Fatal("AddDirEntry failed")
	}
	expectPanic(t, func() { l.AddDirLookupEntry(e) })

	l = NewDirEntryList(buf, 0)
	if l.AddDirLookupEntry(e) == nil {
		t.Fatal("AddDirLookupEntry failed")
	}
	expectPanic(t, func() { l.AddDirEntry(e) })
}
