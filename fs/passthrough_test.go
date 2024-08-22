// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type rwRegisteringNode struct {
	LoopbackNode

	mu     sync.Mutex
	reads  int
	writes int
}

func (n *rwRegisteringNode) Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reads++
	return f.(FileReader).Read(ctx, dest, off)
}

func (n *rwRegisteringNode) Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, errno syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.writes++
	return f.(FileWriter).Write(ctx, data, off)
}

func TestPassthrough(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("passthrough requires CAP_SYS_ADMIN")
	}

	mnt := t.TempDir()
	n := &rwRegisteringNode{}

	rootData := &LoopbackRoot{
		Path: os.TempDir(),
		NewNode: func(rootData *LoopbackRoot, parent *Inode, name string, st *syscall.Stat_t) InodeEmbedder {
			return n
		},
	}
	n.RootData = rootData
	root := &LoopbackNode{
		RootData: rootData,
	}
	opts := &Options{
		OnAdd: func(ctx context.Context) {
			root.AddChild("file",
				root.NewPersistentInode(ctx, n, StableAttr{Mode: syscall.S_IFREG}), false)
		},
	}
	opts.Debug = testutil.VerboseTest()
	server, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	want := "hello there"
	if err := os.WriteFile(mnt+"/file", []byte(want), 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, err := os.ReadFile(mnt + "/file"); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if want != string(got) {
		t.Errorf("got %q want %q", got, want)
	}

	server.Unmount()

	if n.reads > 0 {
		t.Errorf("got readcount %d want 0", n.reads)
	}
	if n.writes > 0 {
		t.Errorf("got writecount %d want 0", n.writes)
	}
}
