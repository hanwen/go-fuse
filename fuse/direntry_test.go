// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"reflect"
	"testing"
)

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

var roundtripEntries = []DirEntry{
	{Name: ".", Ino: 1, Mode: S_IFDIR},
	{Name: "..", Ino: 1, Mode: S_IFDIR},
	{Name: "file", Ino: 42, Mode: S_IFREG},
	{Name: "a-name-of-sixteen", Ino: 43, Mode: S_IFLNK, Off: 100},
	{Name: "12345678", Ino: 44, Mode: S_IFREG},
}

func TestDirEntryListRoundtrip(t *testing.T) {
	buf := make([]byte, 1024)
	for i := range buf {
		buf[i] = 0xff
	}
	l := NewDirEntryList(buf, 10)
	var want []DirEntry
	for _, e := range roundtripEntries {
		if !l.AddDirEntry(e) {
			t.Fatalf("AddDirEntry(%v) failed", e)
		}
		if e.Off == 0 {
			e.Off = l.Offset
		}
		want = append(want, e)
	}

	got, err := ParseDirEntries(l.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if want[1].Off != 12 || want[3].Off != 100 || want[4].Off != 101 {
		t.Errorf("unexpected offsets %v", want)
	}
}

func TestDirEntryListPlusRoundtrip(t *testing.T) {
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = 0xff
	}
	l := NewDirEntryList(buf, 0)
	var want []DirEntry
	var wantOuts []EntryOut
	for i, e := range roundtripEntries {
		out := l.AddDirLookupEntry(e)
		if out == nil {
			t.Fatalf("AddDirLookupEntry(%v) failed", e)
		}
		if *out != (EntryOut{}) {
			t.Errorf("EntryOut not zeroed: %v", out)
		}
		out.NodeId = uint64(i + 100)
		out.Ino = e.Ino
		out.Mode = e.Mode | 0644
		if e.Off == 0 {
			e.Off = l.Offset
		}
		want = append(want, e)
		wantOuts = append(wantOuts, *out)
	}
	l.FixMode(S_IFDIR)
	want[len(want)-1].Mode = S_IFDIR

	got, gotOuts, err := ParseDirLookupEntries(l.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !reflect.DeepEqual(gotOuts, wantOuts) {
		t.Errorf("got %v, want %v", gotOuts, wantOuts)
	}
}

func TestParseDirEntriesTruncated(t *testing.T) {
	buf := make([]byte, 1024)
	l := NewDirEntryList(buf, 0)
	l.AddDirEntry(DirEntry{Name: "file", Ino: 1})
	b := l.Bytes()
	for n := 1; n < len(b); n++ {
		if _, err := ParseDirEntries(b[:n]); err == nil {
			t.Errorf("ParseDirEntries(%d bytes): want error", n)
		}
	}

	l = NewDirEntryList(buf, 0)
	l.AddDirLookupEntry(DirEntry{Name: "file", Ino: 1})
	b = l.Bytes()
	for n := 1; n < len(b); n++ {
		if _, _, err := ParseDirLookupEntries(b[:n]); err == nil {
			t.Errorf("ParseDirLookupEntries(%d bytes): want error", n)
		}
	}
}
