// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"fmt"
	"testing"

	"github.com/hanwen/go-fuse/v3/fs"
)

// TestForget checks that the lookup counts held by the translator
// match what the file system sees, so nodes are forgotten.
func TestForget(t *testing.T) {
	root := &fs.LoopbackNode{RootData: &fs.LoopbackRoot{Path: t.TempDir()}}
	fsys := newTestFS(t, root, nil)
	d, _, st := fsys.create(`\dir`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	fsys.close(d)
	for i := range 10 {
		fsys.createFile(t, fmt.Sprintf(`\dir\f%d`, i), []byte("x"))
	}
	fsys.list(t, `\dir`, 4096)
	for range 3 {
		fsys.getSecurityByName(`\dir\f1`)
	}

	dirInode := root.EmbeddedInode().GetChild("dir")
	child := dirInode.GetChild("f1")
	removed := dirInode.GetChild("f2")
	mustOK(t, "remove", fsys.remove(t, `\dir\f2`))
	if !removed.Forgotten() {
		t.Errorf("removed file is not forgotten")
	}
	if child.Forgotten() {
		t.Fatal("forgotten too early")
	}

	f, _, st := fsys.open(`\dir\f1`, 0)
	mustOK(t, "open", st)
	fsys.tree.ForgetAll()
	if child.Forgotten() {
		t.Errorf("open file was forgotten")
	}
	fsys.close(f)
	if !child.Forgotten() || !dirInode.Forgotten() {
		t.Errorf("not forgotten after close")
	}
	if l := fsys.tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}
}

// TestReadAppend checks that a handle opened for reading and appending
// can do both, on a file system that enforces the open mode.
func TestReadAppend(t *testing.T) {
	root := &fs.LoopbackNode{RootData: &fs.LoopbackRoot{Path: t.TempDir()}}
	fsys := newTestFS(t, root, nil)
	fsys.createFile(t, `\f`, []byte("hello"))

	f, _, st := fsys.open(`\f`, _FILE_READ_DATA|_FILE_APPEND_DATA)
	mustOK(t, "open", st)
	defer fsys.close(f)
	_, _, st = fsys.write(f, []byte(" world"), 0, true, false)
	mustOK(t, "append", st)
	buf := make([]byte, 100)
	n, st := fsys.read(f, buf, 0)
	mustOK(t, "read", st)
	if got := string(buf[:n]); got != "hello world" {
		t.Errorf("got %q", got)
	}
}
