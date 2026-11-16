// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"strings"
	"testing"
	"unsafe"
)

func mustOK(t *testing.T, what string, st ntStatus) {
	t.Helper()
	if st != _STATUS_SUCCESS {
		t.Fatalf("%s: %v", what, st)
	}
}

func TestSymlinkReparseData(t *testing.T) {
	for _, target := range []string{"a", "../dir/file", "dir/", "ünïcødé/😀"} {
		b, st := symlinkReparseData(target)
		mustOK(t, "symlinkReparseData "+target, st)
		got, st := parseSymlinkReparseData(b)
		mustOK(t, "parseSymlinkReparseData "+target, st)
		if got != target {
			t.Errorf("round trip: got %q, want %q", got, target)
		}
	}

	if _, st := symlinkReparseData("/abs/path"); st != _STATUS_ACCESS_DENIED {
		t.Errorf("absolute target: %v", st)
	}
	b, _ := symlinkReparseData("x")
	h := (*symlinkReparseHeader)(unsafe.Pointer(&b[0]))
	h.Flags = 0
	if _, st := parseSymlinkReparseData(b); st != _STATUS_ACCESS_DENIED {
		t.Errorf("absolute reparse point: %v", st)
	}
	if _, st := parseSymlinkReparseData(b[:10]); st != _STATUS_IO_REPARSE_DATA_INVALID {
		t.Errorf("truncated: %v", st)
	}
	h.ReparseTag = 0x80000017 // IO_REPARSE_TAG_WOF
	if _, st := parseSymlinkReparseData(b); st != _STATUS_IO_REPARSE_TAG_MISMATCH {
		t.Errorf("other tag: %v", st)
	}
	if _, st := symlinkReparseData(strings.Repeat("x", maxReparseDataSize)); st != _STATUS_IO_REPARSE_DATA_INVALID {
		t.Errorf("long target: %v", st)
	}
}
