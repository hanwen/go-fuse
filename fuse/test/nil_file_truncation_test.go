// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type truncatableFile struct {
	nodefs.Node
}

func (d *truncatableFile) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	return nil, fuse.OK
}

func (d *truncatableFile) Truncate(file nodefs.File, size uint64, context *fuse.Context) fuse.Status {
	return fuse.OK
}

// TestNilFileTruncation verifies that the FUSE server process does not
// crash when file truncation is performed on nil file handles.
func TestNilFileTruncation(t *testing.T) {
	dir := testutil.TempDir()
	defer func() {
		err := os.Remove(dir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	root := nodefs.NewDefaultNode()
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	srv, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}

	hello := &truncatableFile{
		Node: nodefs.NewDefaultNode(),
	}
	root.Inode().NewChild("hello.txt", false, hello)

	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	defer func() {
		err := srv.Unmount()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// truncate().
	if err := os.Truncate(dir+"/hello.txt", 123); err != nil {
		t.Fatalf("truncate: %s", err)
	}

	// ftruncate().
	f, err := os.OpenFile(dir+"/hello.txt", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open: %s", err)
	}
	defer f.Close()
	if err := f.Truncate(123); err != nil {
		t.Fatalf("ftruncate: %s", err)
	}
}
