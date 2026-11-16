// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package fuseclient

import (
	"archive/zip"
	"context"
	"errors"
	"io"
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

// flagFile returns flags from OPEN, and counts FLUSH.
type flagFile struct {
	gofs.MemRegularFile
	flags   uint32
	flushes *int
}

func (f *flagFile) Open(ctx context.Context, flags uint32) (gofs.FileHandle, uint32, syscall.Errno) {
	return nil, f.flags, 0
}

func (f *flagFile) Flush(ctx context.Context, fh gofs.FileHandle) syscall.Errno {
	*f.flushes++
	return 0
}

// TestFSOpenFlags checks that Handle.Flags returns the FOPEN flags, and
// that the io/fs adapter honors them.
func TestFSOpenFlags(t *testing.T) {
	root := &gofs.MemDir{Attr: fuse.Attr{Mode: 0755}}
	var flushes int
	files := map[string]uint32{
		"plain":   0,
		"noflush": fuse.FOPEN_NOFLUSH | fuse.FOPEN_DIRECT_IO,
		"nonseek": fuse.FOPEN_NONSEEKABLE,
		"stream":  fuse.FOPEN_STREAM,
	}
	fsys, tree := newTestFS(t, root, func(ctx context.Context) {
		r := root.EmbeddedInode()
		for name, flags := range files {
			f := &flagFile{flags: flags, flushes: &flushes}
			f.Data = []byte("hello")
			f.Attr.Mode = 0644
			r.AddChild(name, r.NewPersistentInode(ctx, f, gofs.StableAttr{Mode: fuse.S_IFREG}), false)
		}
	})

	for name, want := range files {
		n, _, _, errno := tree.Walk(&fuse.Caller{}, name)
		if errno != 0 {
			t.Fatal(errno)
		}
		h, errno := tree.Open(&fuse.Caller{}, n.ID, syscall.O_RDONLY)
		if errno != 0 {
			t.Fatal(errno)
		}
		if got := h.Flags() & want; got != want {
			t.Errorf("%s: Flags() = %#x, want %#x set", name, h.Flags(), want)
		}
		tree.Release(h)
		tree.Unref(n)
	}

	open := func(name string) fs.File {
		t.Helper()
		f, err := fsys.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	read2 := func(f fs.File) string {
		t.Helper()
		buf := make([]byte, 2)
		n, err := f.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		return string(buf[:n])
	}

	flushes = 0
	open("plain").Close()
	open("noflush").Close()
	if flushes != 1 {
		t.Errorf("got %d flushes, want 1 (only plain)", flushes)
	}

	for _, name := range []string{"plain", "nonseek", "stream"} {
		f := open(name)
		_, seekErr := f.(io.Seeker).Seek(1, io.SeekStart)
		_, readAtErr := f.(io.ReaderAt).ReadAt(make([]byte, 1), 1)
		f.Close()
		wantErr := name != "plain"
		if (seekErr != nil) != wantErr || (wantErr && !errors.Is(seekErr, syscall.ESPIPE)) {
			t.Errorf("%s: Seek: %v", name, seekErr)
		}
		if (readAtErr != nil) != wantErr || (wantErr && !errors.Is(readAtErr, syscall.ESPIPE)) {
			t.Errorf("%s: ReadAt: %v", name, readAtErr)
		}
	}

	f := open("nonseek")
	if got := read2(f) + read2(f); got != "hell" {
		t.Errorf("nonseek: read %q, want %q", got, "hell")
	}
	f.Close()
	f = open("stream")
	if got := read2(f) + read2(f); got != "hehe" {
		t.Errorf("stream: read %q, want %q (offset 0)", got, "hehe")
	}
	f.Close()
}
