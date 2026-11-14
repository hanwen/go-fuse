// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"context"
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
