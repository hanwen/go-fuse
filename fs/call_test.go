// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type callRecord struct {
	ctx       context.Context
	call      *fuse.Call
	in        fuse.MkdirIn
	inOK      bool
	wrongType bool
	groups    []uint32
	secctx    []fuse.SecCtx
}

type callRecorderNode struct {
	Inode

	mu      sync.Mutex
	records []callRecord
}

var _ = (NodeMkdirer)((*callRecorderNode)(nil))

func (n *callRecorderNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	call, ok := fuse.CallFromContext(ctx)
	if !ok {
		return nil, syscall.EIO
	}
	r := callRecord{ctx: ctx, call: call}
	r.inOK = fuse.Input(call, &r.in)
	var openIn fuse.OpenIn
	r.wrongType = fuse.Input(call, &openIn)
	r.groups, _ = call.SuppGroups()
	r.secctx, _ = call.SecCtx()

	n.mu.Lock()
	n.records = append(n.records, r)
	n.mu.Unlock()

	out.Mode = syscall.S_IFDIR | 0755
	return n.NewInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFDIR}), 0
}

func TestCall(t *testing.T) {
	root := &callRecorderNode{}
	mnt, _ := testMount(t, root, nil)

	if err := os.Mkdir(filepath.Join(mnt, "dir"), 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	root.mu.Lock()
	defer root.mu.Unlock()
	if len(root.records) != 1 {
		t.Fatalf("got %d Mkdir calls, want 1", len(root.records))
	}
	r := root.records[0]
	if !r.inOK {
		t.Errorf("Input(MkdirIn): got false")
	} else if r.in.Mode&0777 != 0750 {
		t.Errorf("MkdirIn.Mode: got %o, want %o", r.in.Mode&0777, 0750)
	}
	if r.wrongType {
		t.Errorf("Input(OpenIn) for MKDIR: got true")
	}
	if got := r.call.Caller().Uid; got != uint32(os.Getuid()) {
		t.Errorf("Caller().Uid: got %d, want %d", got, os.Getuid())
	}

	// The request has finished.
	var in fuse.MkdirIn
	if fuse.Input(r.call, &in) {
		t.Errorf("Input after request: got true")
	}
	if _, ok := r.call.SuppGroups(); ok {
		t.Errorf("SuppGroups after request: got true")
	}
	if got := r.call.Caller().Uid; got != uint32(os.Getuid()) {
		t.Errorf("Caller().Uid after request: got %d, want %d", got, os.Getuid())
	}
	if err := r.ctx.Err(); err != nil {
		t.Errorf("Err after request: %v", err)
	}
}
