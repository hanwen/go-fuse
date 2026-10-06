// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type extRecord struct {
	ok     bool
	groups []uint32
	secctx []fuse.SecCtx
}

type extRecorder struct {
	mu      sync.Mutex
	records map[string][]extRecord
}

func (r *extRecorder) record(ctx context.Context, op string) {
	var rec extRecord
	if call, ok := fuse.CallFromContext(ctx); ok {
		var gok, sok bool
		rec.groups, gok = call.SuppGroups()
		rec.secctx, sok = call.SecCtx()
		rec.ok = gok && sok
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[op] = append(r.records[op], rec)
}

type extNode struct {
	*LoopbackNode
	rec *extRecorder
}

var _ = (NodeWrapChilder)((*extNode)(nil))

func (n *extNode) WrapChild(ctx context.Context, ops InodeEmbedder) InodeEmbedder {
	return &extNode{ops.(*LoopbackNode), n.rec}
}

func (n *extNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.rec.record(ctx, "SYMLINK")
	return n.LoopbackNode.Symlink(ctx, target, name, out)
}

func (n *extNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.rec.record(ctx, "MKDIR")
	return n.LoopbackNode.Mkdir(ctx, name, mode, out)
}

func (n *extNode) Mknod(ctx context.Context, name string, mode, rdev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.rec.record(ctx, "MKNOD")
	return n.LoopbackNode.Mknod(ctx, name, mode, rdev, out)
}

func (n *extNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*Inode, FileHandle, uint32, syscall.Errno) {
	n.rec.record(ctx, "CREATE")
	return n.LoopbackNode.Create(ctx, name, flags, mode, out)
}

type extTestCase struct {
	origDir string
	mntDir  string
	rec     extRecorder
}

func mountExt(t *testing.T, capability uint64, setup func(origDir string)) *extTestCase {
	tc := &extTestCase{
		origDir: t.TempDir(),
		mntDir:  t.TempDir(),
		rec:     extRecorder{records: map[string][]extRecord{}},
	}
	if setup != nil {
		setup(tc.origDir)
	}
	lroot, err := NewLoopbackRoot(tc.origDir)
	if err != nil {
		t.Fatal(err)
	}
	root := &extNode{lroot.(*LoopbackNode), &tc.rec}
	root.RootData.RootNode = root

	opts := &Options{}
	opts.Debug = testutil.VerboseTest()
	opts.ExtraCapabilities = capability

	server, err := Mount(tc.mntDir, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.Unmount()
	})
	if server.KernelSettings().Flags64()&capability == 0 {
		t.Skipf("kernel does not support capability %x", capability)
	}
	return tc
}

func createAll(t *testing.T, tc *extTestCase, subdir string) {
	const target = "some/symlink/target"
	mnt := filepath.Join(tc.mntDir, subdir)
	orig := filepath.Join(tc.origDir, subdir)

	if err := os.Symlink(target, filepath.Join(mnt, "symlink")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := os.Mkdir(filepath.Join(mnt, "dir"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(mnt, "fifo"), 0644); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(mnt, "file"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Close()

	for _, d := range []string{mnt, orig} {
		if got, err := os.Readlink(filepath.Join(d, "symlink")); err != nil {
			t.Errorf("Readlink: %v", err)
		} else if got != target {
			t.Errorf("Readlink(%s): got %q, want %q", d, got, target)
		}
		for name, mode := range map[string]os.FileMode{
			"symlink": os.ModeSymlink,
			"dir":     os.ModeDir,
			"fifo":    os.ModeNamedPipe,
			"file":    0,
		} {
			fi, err := os.Lstat(filepath.Join(d, name))
			if err != nil {
				t.Errorf("Lstat: %v", err)
			} else if fi.Mode().Type() != mode {
				t.Errorf("Lstat(%s/%s): got type %v, want %v", d, name, fi.Mode().Type(), mode)
			}
		}
	}
}

var createOps = []string{"SYMLINK", "MKDIR", "MKNOD", "CREATE"}

func (tc *extTestCase) records(t *testing.T, op string) []extRecord {
	tc.rec.mu.Lock()
	defer tc.rec.mu.Unlock()
	recs := tc.rec.records[op]
	if len(recs) == 0 {
		t.Errorf("no %s requests", op)
	}
	for _, r := range recs {
		if !r.ok {
			t.Errorf("%s: no Call, or Call expired during the request", op)
		}
	}
	return recs
}

func TestExtSuppGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	gid := -1
	for _, g := range groups {
		if g != os.Getegid() {
			gid = g
			break
		}
	}
	if gid < 0 {
		t.Skip("need a supplementary group")
	}

	tc := mountExt(t, fuse.CAP_CREATE_SUPP_GROUP, func(orig string) {
		sub := filepath.Join(orig, "sub")
		if err := os.Mkdir(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(sub, -1, gid); err != nil {
			t.Skipf("Chown to group %d: %v", gid, err)
		}
	})

	createAll(t, tc, "sub")

	want := []uint32{uint32(gid)}
	for _, op := range createOps {
		for _, r := range tc.records(t, op) {
			if !slices.Equal(r.groups, want) {
				t.Errorf("%s: got groups %v, want %v", op, r.groups, want)
			}
		}
	}
}

func lsmActive() bool {
	_, err := os.Stat("/sys/fs/selinux/enforce")
	return err == nil
}

func TestExtSecurityCtx(t *testing.T) {
	tc := mountExt(t, fuse.CAP_SECURITY_CTX, nil)
	createAll(t, tc, "")

	for _, op := range createOps {
		for _, r := range tc.records(t, op) {
			if !lsmActive() {
				continue
			}
			if !slices.ContainsFunc(r.secctx, func(s fuse.SecCtx) bool {
				return s.Name == "security.selinux" && len(s.Value) > 0
			}) {
				t.Errorf("%s: no SELinux context in %v", op, r.secctx)
			}
		}
	}
}
