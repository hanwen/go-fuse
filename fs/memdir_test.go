// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
)

func TestMemDir(t *testing.T) {
	mnt, _ := testMount(t, &MemDir{Attr: fuse.Attr{Mode: 0755}}, nil)

	dir := filepath.Join(mnt, "dir")
	fi, err := os.Stat(mnt)
	if err != nil {
		t.Fatal(err)
	}
	before := fi.ModTime()

	time.Sleep(10 * time.Millisecond)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(mnt); err != nil {
		t.Fatal(err)
	} else if !fi.ModTime().After(before) {
		t.Errorf("mtime of parent not updated: %v", fi.ModTime())
	}

	fi, err = os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode() != os.ModeDir|0700 {
		t.Errorf("mode: got %v, want %v", fi.Mode(), os.ModeDir|0700)
	}
	st := fi.Sys().(*syscall.Stat_t)
	if int(st.Uid) != os.Getuid() || int(st.Gid) != os.Getgid() {
		t.Errorf("owner: got %d:%d, want %d:%d", st.Uid, st.Gid, os.Getuid(), os.Getgid())
	}

	fn := filepath.Join(dir, "file")
	if err := os.WriteFile(fn, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err == nil {
		t.Errorf("rmdir of non-empty directory succeeded")
	}
	if err := os.Rename(fn, filepath.Join(mnt, "moved")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(mnt, "moved")); err != nil {
		t.Fatal(err)
	} else if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
}

type upperDir struct {
	*MemDir
}

var _ = (NodeWrapChilder)((*upperDir)(nil))

func (d *upperDir) WrapChild(ctx context.Context, ops InodeEmbedder) InodeEmbedder {
	switch n := ops.(type) {
	case *MemDir:
		return &upperDir{n}
	case *MemRegularFile:
		return &upperFile{n}
	}
	return ops
}

func (d *upperDir) Unlink(ctx context.Context, name string) syscall.Errno {
	return syscall.EPERM
}

type upperFile struct {
	*MemRegularFile
}

// Open omits FOPEN_KEEP_CACHE: the page cache holds the data as
// written, not as stored.
func (f *upperFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	return nil, 0, 0
}

func (f *upperFile) Write(ctx context.Context, fh FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	return f.MemRegularFile.Write(ctx, fh, bytes.ToUpper(data), off)
}

func TestMemDirWrap(t *testing.T) {
	root := &upperDir{&MemDir{Attr: fuse.Attr{Mode: 0755}}}
	mnt, _ := testMount(t, root, nil)

	dir := filepath.Join(mnt, "a", "b")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	fn := filepath.Join(dir, "file")
	if err := os.WriteFile(fn, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(fn); err != nil {
		t.Fatal(err)
	} else if string(got) != "HELLO" {
		t.Errorf("got %q, want %q", got, "HELLO")
	}
	if err := os.Remove(fn); err == nil {
		t.Errorf("unlink succeeded, want EPERM")
	}
}
