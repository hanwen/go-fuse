// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

func testMount(t *testing.T, root InodeEmbedder, opts *Options) (string, func()) {
	t.Helper()

	mntDir := testutil.TempDir()
	if opts == nil {
		opts = &Options{
			FirstAutomaticIno: 1,
		}
	}
	opts.Debug = testutil.VerboseTest()

	server, err := Mount(mntDir, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	return mntDir, func() {
		server.Unmount()
		os.Remove(mntDir)
	}
}

func TestDataFile(t *testing.T) {
	want := "hello"
	root := &Inode{}
	mntDir, clean := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(
				ctx,
				&MemRegularFile{
					Data: []byte(want),
				},
				NodeAttr{})
			n.AddChild("file", ch, false)
		},
	})
	defer clean()

	fd, err := syscall.Open(mntDir+"/file", syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var buf [1024]byte
	n, err := syscall.Read(fd, buf[:])
	if err != nil {
		t.Errorf("Read: %v", err)
	}

	if err := syscall.Close(fd); err != nil {
		t.Errorf("Close: %v", err)
	}

	got := string(buf[:n])
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

type SymlinkerRoot struct {
	Inode
}

func (s *SymlinkerRoot) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	l := &MemSymlink{
		Data: []byte(target),
	}

	ch := s.NewPersistentInode(ctx, l, NodeAttr{Mode: syscall.S_IFLNK})
	return ch, 0
}

func TestDataSymlink(t *testing.T) {
	root := &SymlinkerRoot{}

	mntDir, clean := testMount(t, root, nil)
	defer clean()

	if err := syscall.Symlink("target", mntDir+"/link"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if got, err := os.Readlink(mntDir + "/link"); err != nil {
		t.Fatalf("Readlink: %v", err)
	} else if want := "target"; got != want {
		t.Errorf("Readlink: got %q want %q", got, want)
	}
}
