// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type randomTypeTest struct {
	Inode
}

var _ = (NodeLookuper)((*randomTypeTest)(nil))
var _ = (NodeReaddirer)((*randomTypeTest)(nil))

// Lookup finds a dir.
func (fn *randomTypeTest) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	stable := StableAttr{
		Mode: fuse.S_IFDIR,
	}

	// Override the file type on a pseudo-random subset of entries
	if crc32.ChecksumIEEE([]byte(name))%2 == 0 {
		stable.Mode = fuse.S_IFREG
	}

	childFN := &randomTypeTest{}
	child := fn.NewInode(ctx, childFN, stable)
	return child, syscall.F_OK
}

// Readdir will always return one child dir.
func (fn *randomTypeTest) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	var entries []fuse.DirEntry

	for i := 0; i < 100; i++ {
		entries = append(entries, fuse.DirEntry{
			Name: fmt.Sprintf("%d", i),
			Mode: fuse.S_IFDIR,
		})
	}
	return NewListDirStream(entries), syscall.F_OK
}

// TestReaddirTypeFixup tests that DirEntryList.FixMode() works as expected.
func TestReaddirTypeFixup(t *testing.T) {
	root := &randomTypeTest{}

	mntDir, _, clean := testMount(t, root, nil)
	defer clean()

	f, err := os.Open(mntDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// (Ab)use loopbackDirStream to call and parse getdents(2) on mntDir.
	// This makes the kernel call READDIRPLUS, which ultimately calls
	// randomTypeTest.Readdir() and randomTypeTest.Lookup() above.
	ds, errno := NewLoopbackDirStream(mntDir)
	if errno != 0 {
		t.Fatalf("readdir: %v", err)
	}
	defer ds.Close()

	for ds.HasNext() {
		e, err := ds.Next()
		if err != 0 {
			t.Errorf("Next: %d", err)
		}
		t.Logf("%q: mode=0x%x", e.Name, e.Mode)
		gotIsDir := (e.Mode & syscall.S_IFDIR) != 0
		wantIsdir := (crc32.ChecksumIEEE([]byte(e.Name)) % 2) == 1
		if gotIsDir != wantIsdir {
			t.Errorf("%q: isdir %v, want %v", e.Name, gotIsDir, wantIsdir)
		}
	}
}
