// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"archive/zip"
	"context"
	"errors"
	"io/fs"
	"syscall"
	"testing"
	"testing/fstest"

	gofs "github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/zipfs"
)

func newTestFS(t *testing.T, root gofs.InodeEmbedder, onAdd func(context.Context)) (*FS, *Tree) {
	t.Helper()
	tree := newTestTree(t, root, onAdd)
	return NewFS(tree, fuse.Caller{}), tree
}

func TestFSMem(t *testing.T) {
	root, onAdd := memTree()
	fsys, tree := newTestFS(t, root, onAdd)
	if err := fstest.TestFS(fsys, "file", "dir/sub/deep", "dir/up", "dirlink", "chain"); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{
		"dir/up":       "hello",
		"dirlink/deep": "deep",
		"chain":        "deep",
	} {
		got, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", name, err)
		} else if string(got) != want {
			t.Errorf("ReadFile(%q): got %q, want %q", name, got, want)
		}
	}

	if fi, err := fsys.Lstat("dirlink/deep"); err != nil {
		t.Errorf("Lstat through symlink: %v", err)
	} else if !fi.Mode().IsRegular() {
		t.Errorf("Lstat(dirlink/deep): got mode %v", fi.Mode())
	}
	if target, err := fsys.ReadLink("chain"); err != nil || target != "dirlink/deep" {
		t.Errorf("ReadLink(chain): got %q, %v", target, err)
	}
	if _, err := fsys.ReadLink("file"); err == nil {
		t.Errorf("ReadLink of regular file succeeded")
	}

	// fstest.TestFS opens all entries, so add the broken links now.
	for name, target := range map[string]string{
		"loop":   "loop",
		"escape": "../outside",
		"abs":    "/etc/passwd",
	} {
		n, _, errno := tree.Symlink(&fsys.caller, tree.Root(), name, target)
		if errno != 0 {
			t.Fatalf("Symlink(%q): %v", name, errno)
		}
		tree.Unref(n)
	}
	for name, want := range map[string]error{
		"loop":         syscall.ELOOP,
		"escape":       syscall.EXDEV,
		"abs":          syscall.EXDEV,
		"file/x":       syscall.ENOTDIR,
		"missing":      fs.ErrNotExist,
		"../file":      fs.ErrInvalid,
		"dir/sub/../x": fs.ErrInvalid,
	} {
		if _, err := fsys.Open(name); !errors.Is(err, want) {
			t.Errorf("Open(%q): got %v, want %v", name, err, want)
		}
	}

	checkForgets(t, tree)
}

func TestFSZip(t *testing.T) {
	const name = "../../zipfs/test.zip"
	root, err := zipfs.NewZipTree(name)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	fsys, tree := newTestFS(t, root, nil)
	var files []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		files = append(files, f.Name)
		want, err := fs.ReadFile(zr, f.Name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := fs.ReadFile(fsys, f.Name)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", f.Name, err)
		} else if string(got) != string(want) {
			t.Errorf("ReadFile(%q): got %q, want %q", f.Name, got, want)
		}
	}
	if err := fstest.TestFS(fsys, files...); err != nil {
		t.Fatal(err)
	}
	checkForgets(t, tree)
}
