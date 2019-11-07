package fuse

import (
	"encoding/hex"
	"syscall"
	"testing"
	"unsafe"
)

// extractDirentplusTyp extracts _Dirent.Typ from the first entry in a
// READDIRPLUS output buffer.
func extractDirentplusTyp(buf []byte) uint32 {
	// "buf" is a READDIRPLUS output buffer and looks like this in memory:
	// 1) fuse.EntryOut
	// 2) fuse._Dirent
	// 3) Name (null-terminated)
	// 4) Padding to align to 8 bytes
	// [repeat]
	off := int(unsafe.Sizeof(EntryOut{})) + int(unsafe.Offsetof(_Dirent{}.Typ))
	_ = buf[off+3] // boundary check
	typ := (*uint32)(unsafe.Pointer(&buf[off]))
	return *typ
}

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
	if have != want {
		t.Errorf("wrong type: %x, want %x", have, want)
		t.Log(hex.Dump(buf))
	}

	// Set mode to regular file and check that
	// "typ" looks like a regular file
	dirents.FixMode(syscall.S_IFREG)
	have = extractDirentplusTyp(buf)
	want = uint32(syscall.S_IFREG) >> 12
	if have != want {
		t.Errorf("wrong type: %x, want %x", have, want)
		t.Log(hex.Dump(buf))
	}
}
