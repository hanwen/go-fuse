//go:build linux
// +build linux

// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type writebackFS struct {
	nodefs.Node
	writeCnt int
}

func newWritebackFS() *writebackFS {
	return &writebackFS{
		Node: nodefs.NewDefaultNode(),
	}
}

func (fs *writebackFS) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	return nil, fuse.OK
}

func (fs *writebackFS) Write(file nodefs.File, data []byte, off int64, context *fuse.Context) (written uint32, code fuse.Status) {
	fs.writeCnt++
	return uint32(len(data)), fuse.OK
}

func TestWriteback(t *testing.T) {
	dir := t.TempDir()

	root := nodefs.NewDefaultNode()
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()

	mountOpts := &fuse.MountOptions{
		EnableWriteback: true,
	}
	if opts != nil && opts.Debug {
		mountOpts.Debug = opts.Debug
	}

	srv, _, err := nodefs.Mount(dir, root, mountOpts, opts)
	if err != nil {
		t.Fatal(err)
	}

	fileName := "writeback.txt"
	node := newWritebackFS()
	root.Inode().NewChild(fileName, false, node)

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

	path := filepath.Join(dir, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile %s: %v", path, err)
	}
	defer f.Close()

	cnt := 2048
	for i := 0; i < cnt; i++ {
		if _, err = f.Write([]byte("1")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if node.writeCnt >= cnt {
		t.Fatalf("write count: got %d; want < %d", node.writeCnt, cnt)
	}
}
