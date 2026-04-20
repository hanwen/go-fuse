// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"log"
	"strings"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type panicNode struct {
	Inode
}

var _ = (NodeSymlinker)((*panicNode)(nil))

func (n *panicNode) Symlink(ctx context.Context, name, target string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	panic("boom")
}

func TestPanic(t *testing.T) {
	root := &panicNode{}
	options := &Options{}
	options.Debug = testutil.VerboseTest()

	buf := &bytes.Buffer{}
	options.MountOptions.Logger = log.New(buf, "", 0)
	dir := t.TempDir()

	rawFS := NewNodeFS(root, options)
	server, err := fuse.NewServer(rawFS, dir, &options.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Symlink("target", dir+"/foo"); err != syscall.EIO {
		t.Fatalf("got %v, want EIO", err)
	}
	server.Unmount()

	if want := "panic in FS handler: boom"; !strings.Contains(buf.String(), want) {
		t.Fatalf("got %s, want %s", buf, want)
	}
}
