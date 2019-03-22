// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type interruptRoot struct {
	DefaultOperations
	child interruptOps
}

type interruptOps struct {
	DefaultOperations
	interrupted bool
}

func (r *interruptRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	if name != "file" {
		return nil, fuse.ENOENT
	}
	ch := InodeOf(r).NewInode(ctx, &r.child, NodeAttr{
		Ino: 2,
		Gen: 1})

	return ch, fuse.OK
}

func (o *interruptOps) Open(ctx context.Context, flags uint32) (FileHandle, uint32, fuse.Status) {
	select {
	case <-time.After(100 * time.Millisecond):
		return nil, 0, fuse.EIO
	case <-ctx.Done():
		o.interrupted = true
		return nil, 0, fuse.EINTR
	}
}

// This currently doesn't test functionality, but is useful to investigate how
// INTERRUPT opcodes are handled.
func TestInterrupt(t *testing.T) {
	mntDir := testutil.TempDir()
	defer os.Remove(mntDir)
	root := &interruptRoot{}

	_ = time.Second
	oneSec := time.Second
	rawFS := NewNodeFS(root, &Options{
		// NOSUBMIT - should run all tests without cache too
		EntryTimeout: &oneSec,
		AttrTimeout:  &oneSec,
	})

	server, err := fuse.NewServer(rawFS, mntDir,
		&fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal(err)
	}

	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	cmd := exec.Command("cat", mntDir+"/file")
	if err := cmd.Start(); err != nil {
		t.Fatalf("run %v: %v", cmd, err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := cmd.Process.Kill(); err != nil {
		t.Errorf("Kill: %v", err)
	}

	server.Unmount()
	if !root.child.interrupted {
		t.Errorf("open request was not interrupted")
	}
}
