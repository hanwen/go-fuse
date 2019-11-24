// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/hanwen/go-fuse/v2/fuse"
)

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

// TestTypeChange simulates inode number reuse that happens on real
// filesystems. For go-fuse, inode number reuse can look like a file changing
// to a directory or vice versa. Acutally, the old inode does not exist anymore,
// we just have not received the FORGET yet.
func TestTypeChange(t *testing.T) {
	rootNode := testTypeChangeIno{}
	mnt, _, clean := testMount(t, &rootNode, nil)
	defer clean()

	for i := 0; i < 100; i++ {
		fi, _ := os.Stat(mnt + "/file")
		syscall.Unlink(mnt + "/file")
		fi, _ = os.Stat(mnt + "/dir")
		if !fi.IsDir() {
			t.Fatal("should be a dir now")
		}
		syscall.Rmdir(mnt + "/dir")
		fi, _ = os.Stat(mnt + "/file")
		if fi.IsDir() {
			t.Fatal("should be a file now")
		}
	}
}

type testTypeChangeIno struct {
	Inode
}

// Lookup function for TestTypeChange: if name == "dir", returns a node of
// type dir, otherwise of type file.
func (fn *testTypeChangeIno) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	mode := uint32(fuse.S_IFREG)
	if name == "dir" {
		mode = fuse.S_IFDIR
	}
	stable := StableAttr{
		Mode: mode,
		Ino:  1234,
	}
	childFN := &testTypeChangeIno{}
	child := fn.NewInode(ctx, childFN, stable)
	return child, syscall.F_OK
}
