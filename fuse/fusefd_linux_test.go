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

func statAsync(path string) chan error {
	ch := make(chan error, 1)
	go func() {
		var st syscall.Stat_t
		ch <- syscall.Stat(path, &st)
	}()
	return ch
}

func expectBlocked(t *testing.T, ch chan error) {
	t.Helper()
	select {
	case err := <-ch:
		t.Fatalf("stat returned %v while the server is stopped", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServerStop(t *testing.T) {
	t.Run("idle", func(t *testing.T) { testServerStop(t, false) })
	t.Run("inflight", func(t *testing.T) { testServerStop(t, true) })
}

// testServerStop checks that requests queue while the server is
// stopped, and are served after Start.
func testServerStop(t *testing.T, inflight bool) {
	fs := newBlockingGetAttrFS(1)
	srv, mnt, served := serveBlockingGetAttr(t, fs)
	if _, err := os.Stat(mnt); err != nil {
		t.Fatal(err)
	}

	var inflightErr chan error
	if inflight {
		inflightErr = statAsync(mnt + "/inflight")
		<-fs.entered
	}

	stopErr := make(chan error, 1)
	go func() { stopErr <- srv.Stop(context.Background()) }()
	if inflight {
		select {
		case err := <-stopErr:
			t.Fatalf("Stop returned %v with a request in flight", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(fs.release)
		if err := <-inflightErr; err != nil {
			t.Errorf("stat: %v", err)
		}
	}
	if err := <-stopErr; err != nil {
		t.Fatalf("Stop: %v", err)
	}

	queued := statAsync(mnt + "/queued")
	expectBlocked(t, queued)
	select {
	case <-served:
		t.Fatal("Serve returned after Stop")
	default:
	}

	if !inflight {
		close(fs.release)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err != nil {
		t.Fatalf("stat after Start: %v", err)
	}
}

// TestServerStopTimeout checks that a server whose Stop timed out can
// be started again.
func TestServerStopTimeout(t *testing.T) {
	fs := newBlockingGetAttrFS(1)
	srv, mnt, _ := serveBlockingGetAttr(t, fs)

	inflightErr := statAsync(mnt + "/inflight")
	<-fs.entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.Stop(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Stop: got %v, want %v", err, context.DeadlineExceeded)
	}
	queued := statAsync(mnt + "/queued")
	expectBlocked(t, queued)

	close(fs.release)
	if err := <-inflightErr; err != nil {
		t.Errorf("stat: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err != nil {
		t.Fatalf("stat after Start: %v", err)
	}
}

func TestServerShutdown(t *testing.T) {
	fs := newBlockingGetAttrFS(1)
	close(fs.release)
	srv, mnt, served := serveBlockingGetAttr(t, fs)
	if _, err := os.Stat(mnt + "/file"); err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
	// No process has the device open, so the kernel aborted the
	// connection.
	var st syscall.Stat_t
	if err := syscall.Stat(mnt+"/file2", &st); err != syscall.ENOTCONN {
		t.Errorf("stat after Shutdown: got %v, want ENOTCONN", err)
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
	want := min(n, r.maxReaders)
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.reqMu.Lock()
		readers := r.reqReaders
		r.reqMu.Unlock()
		if readers >= want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d readers, want >= %d", readers, want)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(mnt + "/after"); err != nil {
		t.Fatalf("stat after idle: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	<-served
}
