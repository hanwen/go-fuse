package fuse

import (
	"encoding/hex"
	"strings"
	"syscall"
	"testing"
)

// TestFixMode tests that DirEntryList.FixMode() works as expected.
func TestFixMode(t *testing.T) {
	buf := make([]byte, 400)
	dirents := NewDirEntryList(buf, 0)
	e := DirEntry{
		Mode: syscall.S_IFDIR,
		Name: "foo",
	}
	dirents.AddDirLookupEntry(e)

	// "typ" should look like a directory
	have := extractDirentplusTyp(buf)
	want := uint32(syscall.S_IFDIR) >> 12
	if *have != want {
		t.Errorf("wrong type: %x, want %x", *have, want)
		t.Log(hex.Dump(buf))
	}

	// Set mode to regular file and check that
	// "typ" looks like a regular file
	dirents.FixMode(syscall.S_IFREG)
	have = extractDirentplusTyp(buf)
	want = uint32(syscall.S_IFREG) >> 12
	if *have != want {
		t.Errorf("wrong type: %x, want %x", *have, want)
		t.Log(hex.Dump(buf))
	}
}

// TestExtractDirentplusTyp tests that extractDirentplusTyp works as expected
func TestExtractDirentplusTyp(t *testing.T) {
	buf := make([]byte, 100000)
	dirents := NewDirEntryList(buf, 0)
	for i := uint32(0); i < 255; i++ {
		// Exercise all possible values (even if they make no sense)
		want := i & 017
		e := DirEntry{
			Mode: want << 12,
			// Exercise all possible name lengths
			Name: strings.Repeat("x", int(i)),
		}
		res := dirents.AddDirLookupEntry(e)
		if res == nil {
			t.Fatal("buf too small")
		}
		have := extractDirentplusTyp(dirents.buf[dirents.lastEntry:])
		if *have != want {
			t.Error()
		}
	}
}
