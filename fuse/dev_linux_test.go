// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v3/internal/testutil"
)

// takeoverFS has a file for every name, all with node ID 2. The file
// size and content are the ID of the file system, so a stat or read
// shows which server answered. Open returns the same file handle in
// every instance, so a handle from one server is valid in the
// next.
type takeoverFS struct {
	defaultRawFileSystem
	id uint64
}

func (fs *takeoverFS) Lookup(ctx context.Context, header *InHeader, name string, out *EntryOut) syscall.Errno {
	out.NodeId = 2
	out.Mode = syscall.S_IFREG | 0o644
	out.Size = fs.id
	return OK
}

func (fs *takeoverFS) GetAttr(ctx context.Context, in *GetAttrIn, out *AttrOut) syscall.Errno {
	if in.NodeId == FUSE_ROOT_ID {
		out.Mode = syscall.S_IFDIR | 0o755
		return OK
	}
	out.Mode = syscall.S_IFREG | 0o644
	out.Size = fs.id
	return OK
}

const takeoverFh = 42

func (fs *takeoverFS) Open(ctx context.Context, in *OpenIn, out *OpenOut) syscall.Errno {
	out.Fh = takeoverFh
	out.OpenFlags = FOPEN_DIRECT_IO
	return OK
}

func (fs *takeoverFS) Read(ctx context.Context, in *ReadIn, buf []byte) (ReadResult, syscall.Errno) {
	if in.Fh != takeoverFh {
		return nil, EBADF
	}
	return ReadResultData([]byte{byte('0' + fs.id)}), OK
}

func statSize(t *testing.T, path string) int64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatalf("stat: %v", err)
	}
	return st.Size
}

// TestTakeover hands a mount from one server to another in the same
// process.
func TestTakeover(t *testing.T) {
	mnt := t.TempDir()
	opts := &MountOptions{Debug: testutil.VerboseTest()}
	a, err := NewServer(&takeoverFS{id: 1}, mnt, opts)
	if err != nil {
		t.Fatal(err)
	}
	aServed := make(chan struct{})
	go func() {
		a.Serve()
		close(aServed)
	}()
	if err := a.WaitMount(); err != nil {
		t.Fatal(err)
	}
	if got := statSize(t, mnt+"/file"); got != 1 {
		t.Fatalf("got size %d, want 1", got)
	}
	f, err := os.Open(mnt + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := a.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, err := a.Dev()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewServerFromDev(&takeoverFS{id: 2}, d, a.mount, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Unmount(); err != nil {
			t.Errorf("Unmount: %v", err)
		}
	})
	if b.KernelSettings().Minor != a.KernelSettings().Minor {
		t.Errorf("got minor %d, want %d", b.KernelSettings().Minor, a.KernelSettings().Minor)
	}

	queued := statAsync(mnt + "/file")
	expectBlocked(t, queued)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := statSize(t, mnt+"/file"); got != 2 {
		t.Fatalf("got size %d, want 2", got)
	}
	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, 0); err != nil {
		t.Fatalf("read file opened before takeover: %v", err)
	}
	if got := string(buf); got != "2" {
		t.Fatalf("read file opened before takeover: got %q, want %q", got, "2")
	}

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-aServed
	if got := statSize(t, mnt+"/file"); got != 2 {
		t.Fatalf("after Shutdown: got size %d, want 2", got)
	}
}

// TestTakeoverAborted resumes the old server when the handoff fails.
func TestTakeoverAborted(t *testing.T) {
	mnt := t.TempDir()
	a, err := NewServer(&takeoverFS{id: 1}, mnt, &MountOptions{Debug: testutil.VerboseTest()})
	if err != nil {
		t.Fatal(err)
	}
	go a.Serve()
	t.Cleanup(func() {
		if err := a.Unmount(); err != nil {
			t.Errorf("Unmount: %v", err)
		}
	})
	if err := a.WaitMount(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, err := a.Dev()
	if err != nil {
		t.Fatal(err)
	}
	d.Close()

	queued := statAsync(mnt + "/file")
	expectBlocked(t, queued)
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err != nil {
		t.Fatalf("stat: %v", err)
	}
}
