// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type interruptRoot struct {
	Inode
	child interruptOps
}

var _ = (NodeLookuper)((*interruptRoot)(nil))

func (r *interruptRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if name != "file" {
		return nil, syscall.ENOENT
	}
	ch := r.Inode.NewInode(ctx, &r.child, StableAttr{
		Ino: 2,
		Gen: 1})

	return ch, OK
}

type interruptOps struct {
	Inode
	interrupted bool
}

var _ = (NodeOpener)((*interruptOps)(nil))

func (o *interruptOps) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	select {
	case <-time.After(100 * time.Millisecond):
		return nil, 0, syscall.EIO
	case <-ctx.Done():
		o.interrupted = true
		return nil, 0, syscall.EINTR
	}
}

// This currently doesn't test functionality, but is useful to investigate how
// INTERRUPT opcodes are handled.
func TestInterrupt(t *testing.T) {
	root := &interruptRoot{}

	oneSec := time.Second
	mntDir, _, clean := testMount(t, root, &Options{
		EntryTimeout: &oneSec,
		AttrTimeout:  &oneSec,
	})
	defer func() {
		if clean != nil {
			clean()
		}
	}()

	cmd := exec.Command("cat", mntDir+"/file")
	if err := cmd.Start(); err != nil {
		t.Fatalf("run %v: %v", cmd, err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := cmd.Process.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}

	clean()
	clean = nil

	if !root.child.interrupted {
		t.Errorf("open request was not interrupted")
	}
}
