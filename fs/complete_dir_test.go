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

// TestCompleteDirEmptyNoNegativeTimeout checks that without a
// NegativeTimeout, an empty directory gets no listing.
func TestCompleteDirEmptyNoNegativeTimeout(t *testing.T) {
	timeout := time.Minute
	mnt, cfs := mountMemCompleteDir(t, &Options{
		EntryTimeout: &timeout,
		AttrTimeout:  &timeout,
	}, 0)

	listDir(t, mnt+"/dir", 0)
	if n := statAbsent(t, cfs, mnt+"/dir", "absent-", 50); n != 50 {
		t.Errorf("got %d LOOKUPs for absent names after listing, want 50", n)
	}
}

// TestCompleteDirNegativeTimeout checks that a listing lasts for
// NegativeTimeout, not for the shorter entry timeout, and that without a
// NegativeTimeout it lasts for the entry timeout.
func TestCompleteDirNegativeTimeout(t *testing.T) {
	entry := 200 * time.Millisecond
	minute := time.Minute
	for _, tc := range []struct {
		name     string
		negative *time.Duration
		expired  bool
	}{
		{"negative", &minute, false},
		{"entry", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mnt, cfs := mountMemCompleteDir(t, &Options{
				EntryTimeout:    &entry,
				AttrTimeout:     &entry,
				NegativeTimeout: tc.negative,
			}, 10)
			listDir(t, mnt+"/dir", 10)
			// A filter for 10 names has 128 bits, so false
			// positives are more likely than for 500 names.
			if n := statAbsent(t, cfs, mnt+"/dir", "absent-", 50); n > completeDirMaxFalsePositives {
				t.Fatalf("got %d LOOKUPs right after listing, want at most %d", n, completeDirMaxFalsePositives)
			}
			time.Sleep(2 * entry)
			n := statAbsent(t, cfs, mnt+"/dir", "absent2-", 50)
			if tc.expired && n != 50 {
				t.Errorf("got %d LOOKUPs after the entry timeout, want 50", n)
			} else if !tc.expired && n > completeDirMaxFalsePositives {
				t.Errorf("got %d LOOKUPs after the entry timeout, want at most %d", n, completeDirMaxFalsePositives)
			}
		})
	}
}

// mountCompleteDir mounts a loopback of a directory holding "dir" with
// nfiles entries, and returns the open "dir" after listing it. The
// open file pins the directory's dentry, and so its listing.
func mountCompleteDir(t *testing.T, nfiles int) (mnt, orig string, cfs *completeDirCountFS, dir *Inode, f *os.File) {
	orig = t.TempDir()
	if err := os.Mkdir(orig+"/dir", 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < nfiles; i++ {
		if err := os.WriteFile(fmt.Sprintf("%s/dir/f-%04d", orig, i), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	timeout := time.Minute
	opts := &Options{
		EntryTimeout: &timeout,
		AttrTimeout:  &timeout,
	}
	opts.Debug = testutil.VerboseTest()
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR

	cfs = &completeDirCountFS{
		RawFileSystem: NewNodeFS(root, opts),
		lookups:       map[string]int{},
	}
	mnt = t.TempDir()
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

	f, err = os.Open(mnt + "/dir")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	es, err := f.ReadDir(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != nfiles {
		t.Fatalf("got %d entries, want %d", len(es), nfiles)
	}
	return mnt, orig, cfs, root.EmbeddedInode().GetChild("dir"), f
}

// Allowance for Bloom filter false positives in 50 stats; the expected
// rate for 500 names is about 0.06%.
const completeDirMaxFalsePositives = 2

func TestCompleteDirLarge(t *testing.T) {
	mnt, orig, cfs, dir, _ := mountCompleteDir(t, 500)

	if n := statAbsent(t, cfs, mnt+"/dir", "absent-", 50); n > completeDirMaxFalsePositives {
		t.Fatalf("got %d LOOKUPs for absent names, want at most %d", n, completeDirMaxFalsePositives)
	}

	// A name added behind the kernel's back is hidden by the listing
	// until NOTIFY_INVAL_ENTRY.
	if err := os.WriteFile(orig+"/dir/remote", nil, 0644); err != nil {
		t.Fatal(err)
	}
	var st syscall.Stat_t
	if err := syscall.Stat(mnt+"/dir/remote", &st); err != syscall.ENOENT {
		t.Fatalf("stat remote before notify: got %v, want ENOENT", err)
	}
	if errno := dir.NotifyEntry("remote"); errno != 0 {
		t.Fatalf("NotifyEntry: %v", errno)
	}
	if err := syscall.Stat(mnt+"/dir/remote", &st); err != nil {
		t.Fatalf("stat remote after notify: %v", err)
	}
	if n := statAbsent(t, cfs, mnt+"/dir", "absent2-", 50); n > completeDirMaxFalsePositives {
		t.Fatalf("got %d LOOKUPs for absent names after NOTIFY_INVAL_ENTRY, want at most %d", n, completeDirMaxFalsePositives)
	}

	// NOTIFY_INVAL_INODE on the directory drops the listing.
	if err := os.WriteFile(orig+"/dir/remote2", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if errno := dir.NotifyContent(0, 0); errno != 0 {
		t.Fatalf("NotifyContent: %v", errno)
	}
	if err := syscall.Stat(mnt+"/dir/remote2", &st); err != nil {
		t.Fatalf("stat remote2 after NOTIFY_INVAL_INODE: %v", err)
	}
	if n := statAbsent(t, cfs, mnt+"/dir", "absent3-", 50); n != 50 {
		t.Errorf("got %d LOOKUPs for absent names after NOTIFY_INVAL_INODE, want 50", n)
	}
}

func dropDentries(t *testing.T) {
	if err := os.WriteFile("/proc/sys/vm/drop_caches", []byte("2"), 0); err != nil {
		t.Fatal(err)
	}
}

// TestCompleteDirDropCaches checks that the listing survives dentry
// reclaim, and that local modifications are reflected in it.
func TestCompleteDirDropCaches(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("need root to drop caches")
	}
	mnt, _, cfs, _, _ := mountCompleteDir(t, 500)
	dropDentries(t)

	var st syscall.Stat_t
	for i := 0; i < 500; i++ {
		if err := syscall.Stat(fmt.Sprintf("%s/dir/f-%04d", mnt, i), &st); err != nil {
			t.Fatalf("stat f-%04d after drop: %v", i, err)
		}
	}
	if n := statAbsent(t, cfs, mnt+"/dir", "absent-", 50); n > completeDirMaxFalsePositives {
		t.Fatalf("got %d LOOKUPs for absent names after drop, want at most %d", n, completeDirMaxFalsePositives)
	}

	if err := os.WriteFile(mnt+"/dir/created", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(mnt+"/dir/f-0001", mnt+"/dir/renamed"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mnt + "/dir/f-0002"); err != nil {
		t.Fatal(err)
	}
	dropDentries(t)

	for _, name := range []string{"created", "renamed", "f-0000"} {
		if err := syscall.Stat(mnt+"/dir/"+name, &st); err != nil {
			t.Errorf("stat %s: %v", name, err)
		}
	}
	for _, name := range []string{"f-0001", "f-0002"} {
		if err := syscall.Stat(mnt+"/dir/"+name, &st); err != syscall.ENOENT {
			t.Errorf("stat %s: got %v, want ENOENT", name, err)
		}
	}
	if n := statAbsent(t, cfs, mnt+"/dir", "absent2-", 50); n > completeDirMaxFalsePositives {
		t.Errorf("got %d LOOKUPs for absent names after modifications, want at most %d", n, completeDirMaxFalsePositives)
	}
}
