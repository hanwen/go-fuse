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

type readdirCountFS struct {
	fuse.RawFileSystem

	mu          sync.Mutex
	lookups     map[string]int
	opendirs    []fuse.Caller
	readdirplus int
	releasedirs int
}

func (c *readdirCountFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	c.mu.Lock()
	c.lookups[name]++
	c.mu.Unlock()
	return c.RawFileSystem.Lookup(ctx, header, name, out)
}

func (c *readdirCountFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	c.mu.Lock()
	c.opendirs = append(c.opendirs, input.Caller)
	c.mu.Unlock()
	return c.RawFileSystem.OpenDir(ctx, input, out)
}

func (c *readdirCountFS) ReadDirPlus(ctx context.Context, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	c.mu.Lock()
	c.readdirplus++
	c.mu.Unlock()
	return c.RawFileSystem.ReadDirPlus(ctx, input, out)
}

func (c *readdirCountFS) ReleaseDir(ctx context.Context, input *fuse.ReleaseIn) {
	c.RawFileSystem.ReleaseDir(ctx, input)
	c.mu.Lock()
	c.releasedirs++
	c.mu.Unlock()
}

func (c *readdirCountFS) lookupCount(prefix string) int {
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

// mountNotify mounts root with CAP_HAS_COMPLETE_DIR. wrap may wrap the
// counting file system. It skips unless the kernel supports
// NOTIFY_READDIRPLUS.
func mountNotify(t *testing.T, root InodeEmbedder, opts *Options, wrap func(*readdirCountFS) fuse.RawFileSystem) (string, *readdirCountFS, *fuse.Server) {
	opts.Debug = testutil.VerboseTest()
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR

	cfs := &readdirCountFS{
		RawFileSystem: NewNodeFS(root, opts),
		lookups:       map[string]int{},
	}
	var rfs fuse.RawFileSystem = cfs
	if wrap != nil {
		rfs = wrap(cfs)
	}
	mnt := t.TempDir()
	server, err := fuse.NewServer(rfs, mnt, &opts.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Unmount() })

	if !server.KernelSettings().SupportsNotify(fuse.NOTIFY_READDIRPLUS) {
		if errno := root.EmbeddedInode().NotifyReaddirplus(); errno != syscall.ENOSYS {
			t.Errorf("NotifyReaddirplus without kernel support: got %v, want ENOSYS", errno)
		}
		t.Skip("kernel does not support NOTIFY_READDIRPLUS")
	}
	return mnt, cfs, server
}

// memDirOptions returns options that add "dir" with nfiles entries
// to root.
func memDirOptions(root, dir *Inode, nfiles int) *Options {
	timeout := time.Minute
	return &Options{
		EntryTimeout: &timeout,
		AttrTimeout:  &timeout,
		OnAdd: func(ctx context.Context) {
			root.AddChild("dir", root.NewPersistentInode(ctx, dir, StableAttr{Mode: syscall.S_IFDIR}), true)
			for i := 0; i < nfiles; i++ {
				ch := dir.NewPersistentInode(ctx, &MemRegularFile{}, StableAttr{})
				dir.AddChild(fmt.Sprintf("f-%04d", i), ch, true)
			}
		},
	}
}

func (c *readdirCountFS) waitReleaseDirs(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		c.mu.Lock()
		done := c.releasedirs >= n
		c.mu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("background READDIRPLUS pass did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNotifyReaddirplus(t *testing.T) {
	root := &Inode{}
	dir := &Inode{}
	mnt, cfs, _ := mountNotify(t, root, memDirOptions(root, dir, 500), nil)

	var st syscall.Stat_t
	if err := syscall.Stat(mnt+"/dir/f-0000", &st); err != nil {
		t.Fatal(err)
	}
	if errno := root.GetChild("dir").GetChild("f-0000").NotifyReaddirplus(); errno != syscall.ENOTDIR {
		t.Errorf("NotifyReaddirplus on a file: got %v, want ENOTDIR", errno)
	}
	if err := syscall.Stat(mnt+"/dir/absent-before", &st); err != syscall.ENOENT {
		t.Fatalf("stat absent-before: got %v, want ENOENT", err)
	}
	if n := cfs.lookupCount("absent-before"); n != 1 {
		t.Fatalf("got %d LOOKUPs for absent-before, want 1", n)
	}

	if errno := dir.NotifyReaddirplus(); errno != 0 {
		t.Fatalf("NotifyReaddirplus: %v", errno)
	}

	cfs.waitReleaseDirs(t, 1)

	cfs.mu.Lock()
	opendirs, readdirplus := cfs.opendirs, cfs.readdirplus
	cfs.mu.Unlock()
	if len(opendirs) != 1 || readdirplus == 0 {
		t.Fatalf("got %d OPENDIR, %d READDIRPLUS, want 1, >0", len(opendirs), readdirplus)
	}
	if got := opendirs[0]; got.Uid != fuse.INVALID_UIDGID || got.Gid != fuse.INVALID_UIDGID {
		t.Errorf("OPENDIR uid/gid: got %d/%d, want INVALID_UIDGID", got.Uid, got.Gid)
	}

	if errno := dir.NotifyReaddirplus(); errno != syscall.EALREADY {
		t.Errorf("NotifyReaddirplus with a valid listing: got %v, want EALREADY", errno)
	}

	for i := 0; i < 100; i++ {
		if err := syscall.Stat(fmt.Sprintf("%s/dir/missing-%d", mnt, i), &st); err != syscall.ENOENT {
			t.Fatalf("stat missing-%d: got %v, want ENOENT", i, err)
		}
	}
	// Bloom filter false positives.
	if n := cfs.lookupCount("missing-"); n > 2 {
		t.Errorf("got %d LOOKUPs for absent names after the pass, want at most 2", n)
	}
	if err := syscall.Stat(mnt+"/dir/f-0499", &st); err != nil {
		t.Errorf("stat f-0499: %v", err)
	}
}

// hintFS sends NOTIFY_READDIRPLUS for the parent from inside negative
// LOOKUP handlers, before the reply.
type hintFS struct {
	*readdirCountFS
	server *fuse.Server

	// beforeHint runs inside the first such handler.
	beforeHint func()
	hints      []syscall.Errno
}

func (h *hintFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	st := h.readdirCountFS.Lookup(ctx, header, name, out)
	// With a NegativeTimeout, negative entries are OK with nodeid 0.
	if st != fuse.ENOENT && !(st.Ok() && out.NodeId == 0) {
		return st
	}
	h.mu.Lock()
	first := len(h.hints) == 0
	h.mu.Unlock()
	if first && h.beforeHint != nil {
		h.beforeHint()
	}
	errno := syscall.Errno(h.server.ReaddirplusNotify(header.NodeId))
	h.mu.Lock()
	h.hints = append(h.hints, errno)
	h.mu.Unlock()
	return st
}

// TestNotifyReaddirplusFromLookup sends the hint from inside a LOOKUP
// handler of a single-threaded server, while a mkdir in the same
// directory waits for the directory lock that the LOOKUP holds.
func TestNotifyReaddirplusFromLookup(t *testing.T) {
	for _, tc := range []struct {
		name     string
		disabled uint64
	}{
		{"parallel", 0},
		{"serial", fuse.CAP_PARALLEL_DIROPS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := t.TempDir()
			if err := os.Mkdir(orig+"/dir", 0755); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 500; i++ {
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
				EntryTimeout:    &timeout,
				AttrTimeout:     &timeout,
				NegativeTimeout: &timeout,
			}
			opts.MountOptions.SingleThreaded = true
			opts.MountOptions.DisabledCapabilities = tc.disabled

			h := &hintFS{}
			mnt, cfs, server := mountNotify(t, root, opts, func(c *readdirCountFS) fuse.RawFileSystem {
				h.readdirCountFS = c
				return h
			})
			h.server = server

			var st syscall.Stat_t
			if err := syscall.Stat(mnt+"/dir", &st); err != nil {
				t.Fatal(err)
			}
			mkdirDone := make(chan error, 1)
			h.beforeHint = func() {
				go func() { mkdirDone <- os.Mkdir(mnt+"/dir/newdir", 0755) }()
				// Let mkdir block on the directory lock.
				time.Sleep(100 * time.Millisecond)
			}
			if err := syscall.Stat(mnt+"/dir/trigger", &st); err != syscall.ENOENT {
				t.Fatalf("stat trigger: got %v, want ENOENT", err)
			}
			select {
			case err := <-mkdirDone:
				if err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("mkdir did not finish")
			}
			h.mu.Lock()
			first := h.hints[0]
			h.mu.Unlock()
			if first != 0 {
				t.Fatalf("NotifyReaddirplus from LOOKUP: got %v, want 0", first)
			}
			cfs.waitReleaseDirs(t, 1)

			for i := 0; i < 50; i++ {
				if err := syscall.Stat(fmt.Sprintf("%s/dir/missing-%d", mnt, i), &st); err != syscall.ENOENT {
					t.Fatalf("stat missing-%d: got %v, want ENOENT", i, err)
				}
			}
			// Bloom filter false positives.
			if n := cfs.lookupCount("missing-"); n > 2 {
				t.Errorf("got %d LOOKUPs for absent names after the pass, want at most 2", n)
			}
			if err := syscall.Stat(mnt+"/dir/newdir", &st); err != nil {
				t.Errorf("stat newdir: %v", err)
			}
		})
	}
}

// denyFS rejects OPENDIR or READDIRPLUS with INVALID_UIDGID.
type denyFS struct {
	*readdirCountFS
	op     string
	denied int
}

func (d *denyFS) deny(op string, caller *fuse.Caller) bool {
	if op != d.op || caller.Uid != fuse.INVALID_UIDGID {
		return false
	}
	d.mu.Lock()
	d.denied++
	d.mu.Unlock()
	return true
}

func (d *denyFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	if d.deny("OPENDIR", &input.Caller) {
		return fuse.EACCES
	}
	return d.readdirCountFS.OpenDir(ctx, input, out)
}

func (d *denyFS) ReadDirPlus(ctx context.Context, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	if d.deny("READDIRPLUS", &input.Caller) {
		return fuse.EACCES
	}
	return d.readdirCountFS.ReadDirPlus(ctx, input, out)
}

// TestNotifyReaddirplusDenied checks that a server that rejects
// INVALID_UIDGID gets no listing.
func TestNotifyReaddirplusDenied(t *testing.T) {
	for _, op := range []string{"OPENDIR", "READDIRPLUS"} {
		t.Run(op, func(t *testing.T) {
			root := &Inode{}
			dir := &Inode{}
			d := &denyFS{op: op}
			mnt, cfs, _ := mountNotify(t, root, memDirOptions(root, dir, 100), func(c *readdirCountFS) fuse.RawFileSystem {
				d.readdirCountFS = c
				return d
			})

			var st syscall.Stat_t
			if err := syscall.Stat(mnt+"/dir", &st); err != nil {
				t.Fatal(err)
			}
			if errno := dir.NotifyReaddirplus(); errno != 0 {
				t.Fatalf("NotifyReaddirplus: %v", errno)
			}
			if op == "READDIRPLUS" {
				cfs.waitReleaseDirs(t, 1)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				cfs.mu.Lock()
				denied := d.denied
				cfs.mu.Unlock()
				if denied > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("no %s with INVALID_UIDGID", op)
				}
				time.Sleep(time.Millisecond)
			}

			for i := 0; i < 50; i++ {
				if err := syscall.Stat(fmt.Sprintf("%s/dir/missing-%d", mnt, i), &st); err != syscall.ENOENT {
					t.Fatalf("stat missing-%d: got %v, want ENOENT", i, err)
				}
			}
			if n := cfs.lookupCount("missing-"); n != 50 {
				t.Errorf("got %d LOOKUPs for absent names after a denied pass, want 50", n)
			}
		})
	}
}
