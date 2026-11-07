// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// blockingGetAttrFS has a file for every name. GETATTR on a file
// blocks until release is closed.
type blockingGetAttrFS struct {
	defaultRawFileSystem

	lastID  atomic.Uint64
	entered chan struct{}
	release chan struct{}
}

func newBlockingGetAttrFS(n int) *blockingGetAttrFS {
	fs := &blockingGetAttrFS{
		entered: make(chan struct{}, n),
		release: make(chan struct{}),
	}
	fs.lastID.Store(FUSE_ROOT_ID)
	return fs
}

func (fs *blockingGetAttrFS) GetAttr(ctx context.Context, in *GetAttrIn, out *AttrOut) Status {
	if in.NodeId == FUSE_ROOT_ID {
		out.Mode = syscall.S_IFDIR | 0o755
		return OK
	}
	fs.entered <- struct{}{}
	<-fs.release
	out.Mode = syscall.S_IFREG | 0o644
	return OK
}

func (fs *blockingGetAttrFS) Lookup(ctx context.Context, header *InHeader, name string, out *EntryOut) Status {
	out.NodeId = fs.lastID.Add(1)
	out.Mode = syscall.S_IFREG | 0o644
	return OK
}

// serveBlockingGetAttr mounts fs, and returns the server, the mount
// point and a channel that is closed when Serve returns.
func serveBlockingGetAttr(t *testing.T, fs *blockingGetAttrFS) (*Server, string, chan struct{}) {
	mnt := t.TempDir()
	srv, err := NewServer(fs, mnt, &MountOptions{Debug: testutil.VerboseTest()})
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() {
		srv.Serve()
		close(served)
	}()
	t.Cleanup(func() {
		if err := srv.Unmount(); err != nil {
			t.Errorf("Unmount: %v", err)
		}
	})
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}
	return srv, mnt, served
}

func TestStopReading(t *testing.T) {
	t.Run("idle", func(t *testing.T) { testStopReading(t, false) })
	t.Run("inflight", func(t *testing.T) { testStopReading(t, true) })
}

func testStopReading(t *testing.T, inflight bool) {
	fs := newBlockingGetAttrFS(1)
	srv, mnt, served := serveBlockingGetAttr(t, fs)

	statErr := make(chan error, 1)
	if inflight {
		go func() {
			var st syscall.Stat_t
			statErr <- syscall.Stat(mnt+"/file", &st)
		}()
		<-fs.entered
	} else if _, err := os.Stat(mnt); err != nil {
		t.Fatal(err)
	}

	srv.fuseFD.stopReading()

	if inflight {
		select {
		case <-served:
			t.Fatal("Serve returned with a request in flight")
		case <-time.After(100 * time.Millisecond):
		}
		close(fs.release)
		if err := <-statErr; err != nil {
			t.Errorf("stat: %v", err)
		}
	}

	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after stopReading")
	}
}

func TestIdleReadersStop(t *testing.T) {
	const n = 4
	fs := newBlockingGetAttrFS(n)
	srv, mnt, served := serveBlockingGetAttr(t, fs)

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			var st syscall.Stat_t
			errs <- syscall.Stat(fmt.Sprintf("%s/file%d", mnt, i), &st)
		}(i)
	}
	for i := 0; i < n; i++ {
		<-fs.entered
	}
	close(fs.release)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("stat: %v", err)
		}
	}

	// The readers that served a request stay, waiting in epoll_wait.
	r := srv.fuseFD
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.reqMu.Lock()
		readers := r.reqReaders
		r.reqMu.Unlock()
		if readers >= n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d readers, want >= %d", readers, n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(mnt + "/after"); err != nil {
		t.Fatalf("stat after idle: %v", err)
	}

	r.stopReading()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after stopReading")
	}
}
