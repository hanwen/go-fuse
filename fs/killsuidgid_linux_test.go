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
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type killRecorder struct {
	mu      sync.Mutex
	records map[string]bool
}

func (r *killRecorder) record(ctx context.Context, op string) {
	call, ok := fuse.CallFromContext(ctx)
	if !ok {
		return
	}
	kill, ok := call.KillSuidgid()
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[op] = kill
}

func (r *killRecorder) get(op string) (killSuidgid, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	killSuidgid, ok = r.records[op]
	return killSuidgid, ok
}

type killNode struct {
	*LoopbackNode
	rec *killRecorder
}

var _ = (NodeWrapChilder)((*killNode)(nil))

func (n *killNode) WrapChild(ctx context.Context, ops InodeEmbedder) InodeEmbedder {
	return &killNode{ops.(*LoopbackNode), n.rec}
}

func (n *killNode) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	n.rec.record(ctx, "OPEN")
	fh, fl, errno := n.LoopbackNode.Open(ctx, flags)
	if errno != 0 {
		return nil, 0, errno
	}
	return &killFile{fh.(*LoopbackFile), n.rec}, fl, 0
}

func (n *killNode) Setattr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	n.rec.record(ctx, "SETATTR")
	if cf, ok := f.(*killFile); ok {
		f = cf.LoopbackFile
	}
	return n.LoopbackNode.Setattr(ctx, f, in, out)
}

type killFile struct {
	*LoopbackFile
	rec *killRecorder
}

func (f *killFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.rec.record(ctx, "WRITE")
	return f.LoopbackFile.Write(ctx, data, off)
}

func mountKillSuidgid(t *testing.T) (orig, mnt string, rec *killRecorder) {
	orig = t.TempDir()
	mnt = t.TempDir()
	rec = &killRecorder{records: map[string]bool{}}
	lroot, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	root := &killNode{lroot.(*LoopbackNode), rec}
	root.RootData.RootNode = root

	opts := &Options{}
	opts.Debug = testutil.VerboseTest()
	// OPEN only carries O_TRUNC (and hence KILL_SUIDGID) with
	// ATOMIC_O_TRUNC; otherwise the kernel truncates via SETATTR.
	opts.ExtraCapabilities = fuse.CAP_HANDLE_KILLPRIV_V2 | fuse.CAP_ATOMIC_O_TRUNC
	server, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Unmount() })
	if server.KernelSettings().Flags64()&fuse.CAP_HANDLE_KILLPRIV_V2 == 0 {
		t.Skip("kernel does not support CAP_HANDLE_KILLPRIV_V2")
	}
	return orig, mnt, rec
}

func TestCallKillSuidgid(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("CAP_FSETID suppresses KILL_SUIDGID")
	}
	orig, mnt, rec := mountKillSuidgid(t)

	// The kernel sets KILL_SUIDGID based on the caller's
	// CAP_FSETID, not on the file mode.
	for _, op := range []string{"WRITE", "SETATTR", "OPEN"} {
		name := "op-" + op + ".txt"
		p := filepath.Join(orig, name)
		if err := os.WriteFile(p, []byte("hello"), 0755); err != nil {
			t.Fatal(err)
		}
		rec.mu.Lock()
		clear(rec.records)
		rec.mu.Unlock()

		p = filepath.Join(mnt, name)
		switch op {
		case "WRITE":
			f, err := os.OpenFile(p, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			f.Close()
			if got, ok := rec.get("OPEN"); !ok || got {
				t.Errorf("OPEN without O_TRUNC: KillSuidgid() = %v, %v, want false, true", got, ok)
			}
		case "SETATTR":
			if err := os.Truncate(p, 1); err != nil {
				t.Fatal(err)
			}
		case "OPEN":
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
		got, ok := rec.get(op)
		if !ok {
			t.Errorf("%s: not called", op)
		} else if !got {
			t.Errorf("%s: KillSuidgid() = false, want true", op)
		}
	}
}
