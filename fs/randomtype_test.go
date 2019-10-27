// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"math/rand"
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
	if rand.Intn(2) == 0 {
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
			Name: "one",
			Mode: fuse.S_IFDIR,
		})
	}
	return NewListDirStream(entries), syscall.F_OK
}

func TestReaddirTypeFixup(t *testing.T) {
	root := &randomTypeTest{}

	mntDir, _, clean := testMount(t, root, nil)
	defer clean()

	f, err := os.Open(mntDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// No panic.
	if _, err := f.Readdir(-1); err != nil {
		t.Fatalf("readdir: %v", err)
	}
}
