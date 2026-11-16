// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"context"
	"maps"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	gofs "github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
)

func newTestTree(t *testing.T, root gofs.InodeEmbedder, onAdd func(context.Context)) *Tree {
	t.Helper()
	opts := &gofs.Options{OnAdd: onAdd}
	opts.Debug = testutil.VerboseTest()
	raw := gofs.NewNodeFS(root, opts)
	c, err := New(raw, &opts.MountOptions, fuse.Notify{})
	if err != nil {
		t.Fatal(err)
	}
	return NewTree(c)
}

// checkForgets checks that dropping the cache returns all lookups.
func checkForgets(t *testing.T, tree *Tree) {
	t.Helper()
	tree.ForgetAll()
	for id, n := range tree.Lookups() {
		if n != 0 {
			t.Errorf("node %d: %d lookups outstanding", id, n)
		}
	}
}

// memTree returns a MemDir root and an OnAdd function that fills it.
func memTree() (*gofs.MemDir, func(context.Context)) {
	root := &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}
	add := func(ctx context.Context, dir *gofs.Inode, name string, ops gofs.InodeEmbedder, mode uint32) *gofs.Inode {
		ch := dir.NewPersistentInode(ctx, ops, gofs.StableAttr{Mode: mode})
		dir.AddChild(name, ch, false)
		return ch
	}
	file := func(data string) gofs.InodeEmbedder {
		return &gofs.MemRegularFile{Data: []byte(data), Attr: fuse.Attr{Mode: 0644}}
	}
	link := func(target string) gofs.InodeEmbedder {
		return &gofs.MemSymlink{Data: []byte(target), Attr: fuse.Attr{Mode: 0777}}
	}
	onAdd := func(ctx context.Context) {
		r := root.EmbeddedInode()
		add(ctx, r, "file", file("hello"), fuse.S_IFREG)
		dir := add(ctx, r, "dir", &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}, fuse.S_IFDIR)
		sub := add(ctx, dir, "sub", &gofs.MemDir{Attr: fuse.Attr{Mode: 0700}}, fuse.S_IFDIR)
		add(ctx, sub, "deep", file("deep"), fuse.S_IFREG)
		add(ctx, dir, "up", link("../file"), fuse.S_IFLNK)
		add(ctx, r, "dirlink", link("dir/sub"), fuse.S_IFLNK)
		add(ctx, r, "chain", link("dirlink/deep"), fuse.S_IFLNK)
	}
	return root, onAdd
}

func TestWalk(t *testing.T) {
	root, onAdd := memTree()
	tree := newTestTree(t, root, onAdd)
	caller := &fuse.Caller{}

	for path, want := range map[string]uint32{
		"":                fuse.S_IFDIR,
		"/":               fuse.S_IFDIR,
		"file":            fuse.S_IFREG,
		"/file":           fuse.S_IFREG,
		"dir//sub///deep": fuse.S_IFREG,
		"dir/sub/":        fuse.S_IFDIR,
		"dirlink":         fuse.S_IFLNK,
		"//dir//up":       fuse.S_IFLNK,
	} {
		n, attr, _, errno := tree.Walk(caller, path)
		if errno != 0 {
			t.Errorf("Walk(%q): %v", path, errno)
			continue
		}
		if got := attr.Mode & S_IFMT; got != want {
			t.Errorf("Walk(%q): got type %o, want %o", path, got, want)
		}
		tree.Unref(n)
	}

	for path, want := range map[string]syscall.Errno{
		"missing":      syscall.ENOENT,
		"file/x":       syscall.ENOTDIR,
		"file/":        syscall.ENOTDIR,
		"dirlink/":     syscall.ENOTDIR,
		"dir/.":        syscall.EINVAL,
		"dir/./sub":    syscall.EINVAL,
		"dir/sub/..":   syscall.EINVAL,
		"..":           syscall.EINVAL,
		"fi\x00le":     syscall.EINVAL,
		"dir/\x00/sub": syscall.EINVAL,
	} {
		if n, _, _, errno := tree.Walk(caller, path); errno != want {
			t.Errorf("Walk(%q): got %v, want %v", path, errno, want)
			if n != nil {
				tree.Unref(n)
			}
		}
	}

	// "." and ".." must not end up in the cache, where they would
	// make Path loop.
	n, _, _, errno := tree.Walk(caller, "dir/sub")
	if errno != 0 {
		t.Fatal(errno)
	}
	done := make(chan string, 1)
	go func() {
		p, _ := tree.Path(n.ID)
		done <- p
	}()
	select {
	case p := <-done:
		if p != "/dir/sub" {
			t.Errorf("Path: got %q, want /dir/sub", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Path does not return")
	}
	tree.Unref(n)

	checkForgets(t, tree)
}

func TestNameWithNUL(t *testing.T) {
	root, onAdd := memTree()
	tree := newTestTree(t, root, onAdd)
	caller := &fuse.Caller{}
	if _, errno := tree.Client.Lookup(caller, fuse.FUSE_ROOT_ID, "file\x00junk"); errno != syscall.EINVAL {
		t.Errorf("Lookup: got %v, want EINVAL", errno)
	}
	if _, _, errno := tree.Create(caller, fuse.FUSE_ROOT_ID, "f\x00", 0, 0644); errno != syscall.EINVAL {
		t.Errorf("Create: got %v, want EINVAL", errno)
	}
	if _, errno := tree.Symlink(caller, fuse.FUSE_ROOT_ID, "l", "t\x00"); errno != syscall.EINVAL {
		t.Errorf("Symlink: got %v, want EINVAL", errno)
	}
	if errno := tree.Rename(caller, fuse.FUSE_ROOT_ID, "file", fuse.FUSE_ROOT_ID, "x\x00"); errno != syscall.EINVAL {
		t.Errorf("Rename: got %v, want EINVAL", errno)
	}
	n, _, _, errno := tree.Walk(caller, "file")
	if errno != 0 {
		t.Fatalf("file is gone: %v", errno)
	}
	tree.Unref(n)
	checkForgets(t, tree)
}

// TestDirAlias checks that a directory has one cache entry, as in the
// kernel's dcache.
func TestDirAlias(t *testing.T) {
	root := &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}
	onAdd := func(ctx context.Context) {
		r := root.EmbeddedInode()
		dir := r.NewPersistentInode(ctx, &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}, gofs.StableAttr{Mode: fuse.S_IFDIR})
		r.AddChild("a", dir, false)
		r.AddChild("b", dir, false)
		dir.AddChild("self", dir, false)
		sub := dir.NewPersistentInode(ctx, &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}, gofs.StableAttr{Mode: fuse.S_IFDIR})
		dir.AddChild("sub", sub, false)
		sub.AddChild("root", r, false)
	}
	tree := newTestTree(t, root, onAdd)
	caller := &fuse.Caller{}

	walk := func(path string) *Node {
		t.Helper()
		n, _, _, errno := tree.Walk(caller, path)
		if errno != 0 {
			t.Fatalf("Walk(%q): %v", path, errno)
		}
		tree.Unref(n)
		return n
	}
	path := func(n *Node) string {
		t.Helper()
		p, ok := tree.Path(n.ID)
		if !ok {
			t.Fatalf("Path(%d) unknown", n.ID)
		}
		return p
	}

	a := walk("a")
	if got := path(a); got != "/a" {
		t.Errorf("Path: got %q, want /a", got)
	}
	if b := walk("b"); b != a {
		t.Fatalf("a and b are different nodes")
	}
	if got := path(a); got != "/b" {
		t.Errorf("Path after lookup of b: got %q, want /b", got)
	}

	// The entry for a moved, so invalidating b leaves no path.
	tree.Invalidate(tree.Root(), "b")
	if p, ok := tree.Path(a.ID); ok {
		t.Errorf("Path after invalidating b: got %q", p)
	}

	a = walk("a")
	for _, p := range []string{"a/self", "a/sub/root"} {
		if _, _, _, errno := tree.Walk(caller, p); errno != syscall.ELOOP {
			t.Errorf("Walk(%q): got %v, want ELOOP", p, errno)
		}
	}
	if got := path(a); got != "/a" {
		t.Errorf("Path after loops: got %q, want /a", got)
	}

	checkForgets(t, tree)
}

func TestWalkFollow(t *testing.T) {
	root, onAdd := memTree()
	tree := newTestTree(t, root, onAdd)
	caller := &fuse.Caller{}

	for path, want := range map[string]uint32{
		"":                    fuse.S_IFDIR,
		"/":                   fuse.S_IFDIR,
		".":                   fuse.S_IFDIR,
		"dir/sub/../..":       fuse.S_IFDIR,
		"dir//up":             fuse.S_IFREG,
		"dir/sub/../up":       fuse.S_IFREG,
		"dirlink/":            fuse.S_IFDIR,
		"dirlink/./deep":      fuse.S_IFREG,
		"chain":               fuse.S_IFREG,
		"dirlink/../sub/deep": fuse.S_IFREG,
	} {
		n, attr, errno := tree.WalkFollow(caller, path)
		if errno != 0 {
			t.Errorf("WalkFollow(%q): %v", path, errno)
			continue
		}
		if got := attr.Mode & S_IFMT; got != want {
			t.Errorf("WalkFollow(%q): got type %o, want %o", path, got, want)
		}
		tree.Unref(n)
	}

	for path, want := range map[string]syscall.Errno{
		"missing":     syscall.ENOENT,
		"file/":       syscall.ENOTDIR,
		"chain/":      syscall.ENOTDIR,
		"dir/up/":     syscall.ENOTDIR,
		"file/x":      syscall.ENOTDIR,
		"..":          syscall.EXDEV,
		"dir/../..":   syscall.EXDEV,
		"dir/\x00":    syscall.EINVAL,
		"dir/sub/../": 0,
	} {
		n, _, errno := tree.WalkFollow(caller, path)
		if errno != want {
			t.Errorf("WalkFollow(%q): got %v, want %v", path, errno, want)
		}
		if errno == 0 {
			tree.Unref(n)
		}
	}

	checkForgets(t, tree)
}

// releaseCounter counts RELEASE, RELEASEDIR, FORGET and OnUnmount.
type releaseCounter struct {
	fuse.RawFileSystem

	mu                sync.Mutex
	releases, relDirs int
	unmounts          int
	forgot            map[uint64]uint64
}

func (c *releaseCounter) Forget(nodeid, nlookup uint64) {
	c.mu.Lock()
	c.forgot[nodeid] += nlookup
	c.mu.Unlock()
	c.RawFileSystem.Forget(nodeid, nlookup)
}

func (c *releaseCounter) Release(ctx context.Context, in *fuse.ReleaseIn) {
	c.mu.Lock()
	c.releases++
	c.mu.Unlock()
	c.RawFileSystem.Release(ctx, in)
}

func (c *releaseCounter) ReleaseDir(ctx context.Context, in *fuse.ReleaseIn) {
	c.mu.Lock()
	c.relDirs++
	c.mu.Unlock()
	c.RawFileSystem.ReleaseDir(ctx, in)
}

func (c *releaseCounter) OnUnmount() {
	c.mu.Lock()
	c.unmounts++
	c.mu.Unlock()
	c.RawFileSystem.OnUnmount()
}

// TestClose checks that Close releases the handles that are still
// open, but not those that were released, forgets all nodes, and
// unmounts once.
func TestClose(t *testing.T) {
	root, onAdd := memTree()
	c := &releaseCounter{
		RawFileSystem: gofs.NewNodeFS(root, &gofs.Options{OnAdd: onAdd}),
		forgot:        map[uint64]uint64{},
	}
	cl, err := New(c, &fuse.MountOptions{Debug: testutil.VerboseTest()}, fuse.Notify{})
	if err != nil {
		t.Fatal(err)
	}
	tree := NewTree(cl)
	fsys := NewFS(tree, fuse.Caller{})

	closed, err := fsys.Open("file")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Open("file"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Open("dir/sub/deep"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Open("dir"); err != nil {
		t.Fatal(err)
	}
	c.releases, c.relDirs = 0, 0
	c.forgot = map[uint64]uint64{}
	lookups := tree.Lookups()
	if len(lookups) == 0 {
		t.Fatal("no lookups")
	}

	tree.Close()
	tree.Close()
	if c.releases != 2 || c.relDirs != 1 {
		t.Errorf("got %d releases, %d releasedirs; want 2, 1", c.releases, c.relDirs)
	}
	if c.unmounts != 1 {
		t.Errorf("OnUnmount called %d times, want 1", c.unmounts)
	}
	if l := tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}
	if !maps.Equal(c.forgot, lookups) {
		t.Errorf("forgot %v, want %v", c.forgot, lookups)
	}
}

// constFh returns the same file handle for every open.
type constFh struct {
	fuse.RawFileSystem

	mu           sync.Mutex
	releases     int
	releaseFlags []uint32
}

func (c *constFh) Create(ctx context.Context, in *fuse.CreateIn, name string, out *fuse.CreateOut) syscall.Errno {
	out.NodeId = 2
	out.Fh = 7
	return 0
}

func (c *constFh) Open(ctx context.Context, in *fuse.OpenIn, out *fuse.OpenOut) syscall.Errno {
	out.Fh = 7
	return 0
}

func (c *constFh) Release(ctx context.Context, in *fuse.ReleaseIn) {
	c.mu.Lock()
	c.releases++
	c.releaseFlags = append(c.releaseFlags, in.Flags)
	c.mu.Unlock()
}

// TestReleaseFlags checks that RELEASE has the open flags without the
// ones that only apply while opening, as from the kernel.
func TestReleaseFlags(t *testing.T) {
	raw := &constFh{RawFileSystem: fuse.NewDefaultRawFileSystem()}
	c, err := New(raw, &fuse.MountOptions{Debug: testutil.VerboseTest()}, fuse.Notify{})
	if err != nil {
		t.Fatal(err)
	}
	_, h, errno := c.Create(nil, fuse.FUSE_ROOT_ID, "f", syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR|syscall.O_APPEND, 0644)
	if errno != 0 {
		t.Fatal(errno)
	}
	c.Release(h)
	h, errno = c.Open(nil, 2, syscall.O_WRONLY|syscall.O_TRUNC|syscall.O_NOCTTY)
	if errno != 0 {
		t.Fatal(errno)
	}
	c.Release(h)
	want := []uint32{syscall.O_RDWR | syscall.O_APPEND, syscall.O_WRONLY}
	if !slices.Equal(raw.releaseFlags, want) {
		t.Errorf("RELEASE flags %#o, want %#o", raw.releaseFlags, want)
	}
}

// TestSameFh checks that handles are tracked per open, not by the
// file system's file handle, which is opaque and may be reused.
func TestSameFh(t *testing.T) {
	raw := &constFh{RawFileSystem: fuse.NewDefaultRawFileSystem()}
	c, err := New(raw, &fuse.MountOptions{Debug: testutil.VerboseTest()}, fuse.Notify{})
	if err != nil {
		t.Fatal(err)
	}
	var hs []*Handle
	for _, flags := range []uint32{syscall.O_RDONLY, syscall.O_RDWR, syscall.O_RDONLY} {
		h, errno := c.Open(nil, fuse.FUSE_ROOT_ID, flags)
		if errno != 0 {
			t.Fatal(errno)
		}
		hs = append(hs, h)
	}
	c.Release(hs[0])
	c.Release(hs[0])
	if raw.releases != 1 {
		t.Errorf("got %d releases, want 1", raw.releases)
	}
	c.Close()
	if raw.releases != 3 {
		t.Errorf("got %d releases after Close, want 3", raw.releases)
	}
	c.Release(hs[1])
	if raw.releases != 3 {
		t.Errorf("release after Close sent RELEASE")
	}
}

// TestAfterClose checks that requests after Close fail with ENOTCONN.
func TestAfterClose(t *testing.T) {
	root, onAdd := memTree()
	tree := newTestTree(t, root, onAdd)
	tree.Close()
	if _, errno := tree.Getattr(&fuse.Caller{}, fuse.FUSE_ROOT_ID, nil); errno != syscall.ENOTCONN {
		t.Errorf("Getattr: got %v, want ENOTCONN", errno)
	}
}
