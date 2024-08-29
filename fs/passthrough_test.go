// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"io"
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
		Path: t.TempDir(),
		NewNode: func(rootData *LoopbackRoot, parent *Inode, name string, st *syscall.Stat_t) InodeEmbedder {
			return n
		},
	}
	n.RootData = rootData
	root := &LoopbackNode{
		RootData: rootData,
	}
	opts := &Options{}
	opts.Debug = testutil.VerboseTest()
	server, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	if 0 == server.KernelSettings().Flags64()&fuse.CAP_PASSTHROUGH {
		t.Skip("Kernel does not support passthrough")
	}
	fn := mnt + "/file"
	want := "hello there"
	if err := os.WriteFile(fn, []byte(want), 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if want != string(got) {
		t.Errorf("got %q want %q", got, want)
	}

	want2 := "xxxx"
	if err := os.WriteFile(fn, []byte(want2), 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got2, err := os.ReadFile(fn)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got2) != want2 {
		t.Errorf("got %q want %q", got2, want2)
	}

	f.Close()
	server.Unmount()

	if n.reads > 0 {
		t.Errorf("got readcount %d want 0", n.reads)
	}
	if n.writes > 0 {
		t.Errorf("got writecount %d want 0", n.writes)
	}
}
