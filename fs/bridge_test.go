// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"encoding/hex"
	"testing"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type _Dirent struct {
	Ino     uint64
	Off     uint64
	NameLen uint32
	Typ     uint32
}

// TestBridgeReaddirPlusVirtualEntries looks at "." and ".." in the ReadDirPlus
// output. They should exist, but the NodeId should be zero.
func TestBridgeReaddirPlusVirtualEntries(t *testing.T) {
	// Set suppressDebug as we do our own logging
	tc := newTestCase(t, &testOptions{suppressDebug: true})
	defer tc.Clean()

	rb := tc.rawFS.(*rawBridge)

	// We only populate what rawBridge.OpenDir() actually looks at.
	openIn := fuse.OpenIn{}
	openIn.NodeId = 1 // root node always has id 1 and always exists
	openOut := fuse.OpenOut{}
	status := rb.OpenDir(nil, &openIn, &openOut)
	if !status.Ok() {
		t.Fatal(status)
	}

	// We only populate what rawBridge.ReadDirPlus() actually looks at.
	readIn := fuse.ReadIn{}
	readIn.NodeId = 1
	readIn.Fh = openOut.Fh
	buf := make([]byte, 400)
	dirents := fuse.NewDirEntryList(buf, 0)
	status = rb.ReadDirPlus(nil, &readIn, dirents)
	if !status.Ok() {
		t.Fatal(status)
	}
	t.Logf("buf=\n%s", hex.Dump(buf))

	// Parse the output buffer. Looks like this in memory:
	// 1) fuse.EntryOut
	// 2) fuse._Dirent
	// 3) Name (null-terminated)
	// 4) Padding to align to 8 bytes
	// [repeat]
	const entryOutSize = int(unsafe.Sizeof(fuse.EntryOut{}))
	// = unsafe.Sizeof(fuse._Dirent{}), see fuse/types.go
	const direntSize = 24
	// Round up to 8.
	const entry2off = (entryOutSize + direntSize + len(".\x00") + 7) / 8 * 8

	// 1st entry should be "."
	entry1 := (*fuse.EntryOut)(unsafe.Pointer(&buf[0]))
	name1 := string(buf[entryOutSize+direntSize : entryOutSize+direntSize+2])
	if name1 != ".\x00" {
		t.Errorf("Unexpected 1st entry %q", name1)
	}
	t.Logf("entry1 %q: %#v", name1, entry1)

	// 2nd entry should be ".."
	entry2 := (*fuse.EntryOut)(unsafe.Pointer(&buf[entry2off]))
	name2 := string(buf[entry2off+entryOutSize+direntSize : entry2off+entryOutSize+direntSize+2])
	if name2 != ".." {
		t.Errorf("Unexpected 2nd entry %q", name2)
	}
	t.Logf("entry2 %q: %#v", name2, entry2)

	if entry1.NodeId != 0 {
		t.Errorf("entry1 NodeId should be 0, but is %d", entry1.NodeId)
	}
	if entry2.NodeId != 0 {
		t.Errorf("entry2 NodeId should be 0, but is %d", entry2.NodeId)
	}
}
