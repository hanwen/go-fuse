// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package fs

import (
	"bytes"
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/splice"
)

type pipefailNode struct {
	Inode
	promise int64
	actual  []byte
}

func (n *pipefailNode) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *pipefailNode) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0444
	out.Size = uint64(n.promise)
	return 0
}

func (n *pipefailNode) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off >= n.promise {
		return fuse.ReadResultData(nil), 0
	}

	// Total bytes to serve, clamped to file size and dest buffer.
	end := min(off+int64(len(dest)), n.promise)
	total := int(end - off)

	pair, err := splice.Get()
	if err != nil {
		return nil, syscall.EIO
	}

	if err := pair.Grow(total); err != nil {
		return nil, syscall.EIO
	}

	pair.Write(n.actual)

	// simulate write failure
	return fuse.ReadResultPipe(pair, total), 0
}

func TestPipeFail(t *testing.T) {
	root := &Inode{}
	pf := &pipefailNode{
		actual:  bytes.Repeat([]byte{42}, 50),
		promise: 100,
	}

	opts := &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(ctx, pf, StableAttr{})
			n.AddChild("pipefail", ch, false)
		},
	}
	sec := time.Second
	opts.EntryTimeout = &sec
	opts.AttrTimeout = &sec
	opts.Debug = testutil.VerboseTest()
	mntDir := t.TempDir()
	server, err := Mount(mntDir, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	got, err := os.ReadFile(mntDir + "/pipefail")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pf.actual) {
		t.Fatalf("content mismatch: got %d bytes, want %d\n", len(got), len(pf.actual))
	}
}
