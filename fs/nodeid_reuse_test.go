// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
)

type reuseFileHandle struct {
	data []byte
}

var _ = (fs.FileReader)((*reuseFileHandle)(nil))

func (fh *reuseFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	off = min(off, int64(len(fh.data)))
	end := min(int(off)+len(dest), len(fh.data))
	return fuse.ReadResultData(fh.data[off:end]), 0
}

type reuseFile struct {
	fs.Inode
	data []byte
}

var _ = (fs.NodeOpener)((*reuseFile)(nil))

func (f *reuseFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return &reuseFileHandle{data: f.data}, 0, 0
}

var _ = (fs.NodeGetattrer)((*reuseFile)(nil))

func (f *reuseFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr.Size = uint64(len(f.data))
	return 0
}

type nodeIDReuseRoot struct {
	fs.Inode
}

var _ = (fs.NodeLookuper)((*nodeIDReuseRoot)(nil))

func (n *nodeIDReuseRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var gen uint64
	var data string
	switch name {
	case "a":
		gen, data = 1, "old-generation-data"
	case "b":
		gen, data = 2, "new-generation-data"
	default:
		return nil, syscall.ENOENT
	}
	ops := &reuseFile{data: []byte(data)}
	child := n.NewInode(ctx, ops, fs.StableAttr{Mode: syscall.S_IFREG, Ino: 100, Gen: gen})
	return child, 0
}

func TestNodeIDReuseStaleInode(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test needs linux")
	}

	root := &nodeIDReuseRoot{}
	dir := t.TempDir()
	opts := &fs.Options{ExternalNodeID: true}
	opts.Debug = testutil.VerboseTest()
	server, err := fs.Mount(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	// Open "a" and keep the fd around across the lookup of "b" below -
	// this is the "old file descriptor" whose fate we want to observe.
	fdA, err := os.Open(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	defer fdA.Close()

	bufA1 := make([]byte, 64)
	nA1, err := fdA.Read(bufA1)
	if err != nil {
		t.Fatalf("Read(fdA): %v", err)
	}
	if got := string(bufA1[:nA1]); got != "old-generation-data" {
		t.Fatalf("initial read of a: got %q", got)
	}

	// Open "b": an entirely different file (different name, different
	// generation, different content), but - because it shares "a"'s Ino
	// under ExternalNodeID - it is handed the very same nodeid.
	fdB, err := os.Open(filepath.Join(dir, "b"))
	if err != nil {
		t.Fatal(err)
	}
	defer fdB.Close()

	bufB := make([]byte, 64)
	nB, err := fdB.Read(bufB)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(bufB[:nB]); got != "new-generation-data" {
		t.Fatalf("reading %q returned %q, want b's own content %q", "b", got, "new-generation-data")
	}

	if _, err := fdA.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	bufA2 := make([]byte, 64)
	_, err = fdA.Read(bufA2)
	if !errors.Is(err, syscall.EIO) {
		t.Fatalf("got %#v, want EIO", err)
	}
}
