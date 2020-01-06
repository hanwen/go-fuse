// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"
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
	releaseIn := fuse.ReleaseIn{
		Fh: openOut.Fh,
	}
	releaseIn.NodeId = 1
	defer rb.ReleaseDir(&releaseIn)

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

	names := map[string]*fuse.EntryOut{}
	// 1st entry should be "."
	entry1 := (*fuse.EntryOut)(unsafe.Pointer(&buf[0]))
	name1 := string(buf[entryOutSize+direntSize : entryOutSize+direntSize+2])
	names[name1] = entry1

	// 2nd entry should be ".."
	entry2 := (*fuse.EntryOut)(unsafe.Pointer(&buf[entry2off]))
	name2 := string(buf[entry2off+entryOutSize+direntSize : entry2off+entryOutSize+direntSize+2])

	names[name2] = entry2

	if len(names) != 2 || names[".\000"] == nil || names[".."] == nil {
		t.Fatalf(`got %v, want {".\\0", ".."}`, names)
	}

	for k, v := range names {
		if v.NodeId != 0 {
			t.Errorf("entry %q NodeId should be 0, but is %d", k, v.NodeId)
		}
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

// Lookup function for TestTypeChange:
// If name == "dir", returns a node of type dir,
// if name == "file" of type file,
// otherwise ENOENT.
func (fn *testTypeChangeIno) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	var mode uint32
	switch name {
	case "file":
		mode = syscall.S_IFREG
	case "dir":
		mode = syscall.S_IFDIR
	default:
		return nil, syscall.ENOENT
	}
	stable := StableAttr{
		Mode: mode,
		Ino:  1234,
	}
	childFN := &testTypeChangeIno{}
	child := fn.NewInode(ctx, childFN, stable)
	return child, syscall.F_OK
}

// TestDeletedInodePath checks that Inode.Path returns ".deleted" if an Inode is
// disconnected from the hierarchy (=orphaned)
func TestDeletedInodePath(t *testing.T) {
	rootNode := testDeletedIno{}
	mnt, _, clean := testMount(t, &rootNode, &Options{Logger: log.New(os.Stderr, "", 0)})
	defer clean()

	// Open a file handle so the kernel cannot FORGET the inode
	fd, err := os.Open(mnt + "/dir")
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	// Delete it so the inode does not have a path anymore
	err = syscall.Rmdir(mnt + "/dir")
	if err != nil {
		t.Fatal(err)
	}
	atomic.StoreInt32(&rootNode.deleted, 1)

	// Our Getattr implementation `testDeletedIno.Getattr` should return
	// ENFILE when everything looks ok, EILSEQ otherwise.
	var st syscall.Stat_t
	err = syscall.Fstat(int(fd.Fd()), &st)
	if err != syscall.ENFILE {
		t.Error(err)
	}
}

type testDeletedIno struct {
	Inode

	deleted int32
}

func (n *testDeletedIno) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	ino := n.Root().Operations().(*testDeletedIno)
	if atomic.LoadInt32(&ino.deleted) == 1 {
		return nil, syscall.ENOENT
	}
	if name != "dir" {
		return nil, syscall.ENOENT
	}
	childNode := &testDeletedIno{}
	stable := StableAttr{Mode: syscall.S_IFDIR, Ino: 999}
	child := n.NewInode(ctx, childNode, stable)
	return child, syscall.F_OK
}

func (n *testDeletedIno) Opendir(ctx context.Context) syscall.Errno {
	return OK
}

func (n *testDeletedIno) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	prefix := ".go-fuse"
	p := n.Path(n.Root())
	if strings.HasPrefix(p, prefix) {
		// Return ENFILE when things look ok
		return syscall.ENFILE
	}
	// Otherwise EILSEQ
	return syscall.EILSEQ
}
