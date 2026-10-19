// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// completeDirCountFS sets FOPEN_COMPLETE_DIR on all directories and
// counts LOOKUPs per name.
type completeDirCountFS struct {
	fuse.RawFileSystem

	mu      sync.Mutex
	lookups map[string]int
}

func (c *completeDirCountFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	c.mu.Lock()
	c.lookups[name]++
	c.mu.Unlock()
	return c.RawFileSystem.Lookup(ctx, header, name, out)
}

func (c *completeDirCountFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	st := c.RawFileSystem.OpenDir(ctx, input, out)
	if st.Ok() {
		out.OpenFlags |= fuse.FOPEN_COMPLETE_DIR
	}
	return st
}

func (c *completeDirCountFS) lookupCount(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k, v := range c.lookups {
		if strings.HasPrefix(k, prefix) {
			n += v
		}
	}
	return n
}

// mountMemCompleteDir mounts an in-memory directory "dir" with nfiles
// entries, with FOPEN_COMPLETE_DIR set on all directories.
func mountMemCompleteDir(t *testing.T, opts *Options, nfiles int) (string, *completeDirCountFS) {
	root := &Inode{}
	opts.OnAdd = func(ctx context.Context) {
		dir := root.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFDIR})
		root.AddChild("dir", dir, true)
		for i := 0; i < nfiles; i++ {
			dir.AddChild(fmt.Sprintf("f-%04d", i), dir.NewPersistentInode(ctx, &MemRegularFile{}, StableAttr{}), true)
		}
	}
	opts.Debug = testutil.VerboseTest()
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR

	cfs := &completeDirCountFS{
		RawFileSystem: NewNodeFS(root, opts),
		lookups:       map[string]int{},
	}
	mnt := t.TempDir()
	server, err := fuse.NewServer(cfs, mnt, &opts.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Unmount() })

	if server.KernelSettings().Flags64()&fuse.CAP_HAS_COMPLETE_DIR == 0 {
		t.Skip("kernel does not support CAP_HAS_COMPLETE_DIR")
	}
	return mnt, cfs
}

func listDir(t *testing.T, dir string, want int) {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != want {
		t.Fatalf("got %d entries, want %d", len(es), want)
	}
}

// statAbsent stats n fresh names with the given prefix, and returns the
// number of LOOKUPs they caused.
func statAbsent(t *testing.T, cfs *completeDirCountFS, dir, prefix string, n int) int {
	t.Helper()
	before := cfs.lookupCount(prefix)
	var st syscall.Stat_t
	for i := 0; i < n; i++ {
		if err := syscall.Stat(fmt.Sprintf("%s/%s%d", dir, prefix, i), &st); err != syscall.ENOENT {
			t.Fatalf("stat %s%d: got %v, want ENOENT", prefix, i, err)
		}
	}
	return cfs.lookupCount(prefix) - before
}

// TestCompleteDirEmpty lists an empty directory with FOPEN_COMPLETE_DIR.
// Only "." carries an entry timeout (NegativeTimeout), so it alone
// makes the listing valid.
func TestCompleteDirEmpty(t *testing.T) {
	timeout := time.Minute
	mnt, cfs := mountMemCompleteDir(t, &Options{
		EntryTimeout:    &timeout,
		AttrTimeout:     &timeout,
		NegativeTimeout: &timeout,
	}, 0)

	var st syscall.Stat_t
	if err := syscall.Stat(mnt+"/dir/absent-before", &st); err != syscall.ENOENT {
		t.Fatalf("stat absent-before: got %v, want ENOENT", err)
	}
	if n := cfs.lookupCount("absent-before"); n != 1 {
		t.Fatalf("got %d LOOKUPs for absent-before, want 1", n)
	}

	listDir(t, mnt+"/dir", 0)
	// The filter for an empty listing has no bits set, so there are
	// no false positives.
	if n := statAbsent(t, cfs, mnt+"/dir", "absent-", 50); n != 0 {
		t.Errorf("got %d LOOKUPs for absent names after listing, want 0", n)
	}
}
