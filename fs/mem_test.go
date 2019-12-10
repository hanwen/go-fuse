// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func testMount(t *testing.T, root InodeEmbedder, opts *Options) (string, *fuse.Server, func()) {
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
	return mntDir, server, func() {
		if err := server.Unmount(); err != nil {
			t.Fatalf("testMount: Unmount failed: %v", err)
		}
		if err := syscall.Rmdir(mntDir); err != nil {
			t.Errorf("testMount: Remove failed: %v", err)
		}
	}
}

func TestDefaultOwner(t *testing.T) {
	want := "hello"
	root := &Inode{}
	mntDir, _, clean := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(
				ctx,
				&MemRegularFile{
					Data: []byte(want),
				},
				StableAttr{})
			n.AddChild("file", ch, false)
		},
		UID: 42,
		GID: 43,
	})
	defer clean()

	var st syscall.Stat_t
	if err := syscall.Lstat(mntDir+"/file", &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	} else if st.Uid != 42 || st.Gid != 43 {
		t.Fatalf("Got Lstat %d, %d want 42,43", st.Uid, st.Gid)
	}
}

func TestDataFile(t *testing.T) {
	want := "hello"
	root := &Inode{}
	mntDir, _, clean := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(
				ctx,
				&MemRegularFile{
					Data: []byte(want),
					Attr: fuse.Attr{
						Mode: 0464,
					},
				},
				StableAttr{})
			n.AddChild("file", ch, false)
		},
	})
	defer clean()

	var st syscall.Stat_t
	if err := syscall.Lstat(mntDir+"/file", &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if want := uint32(syscall.S_IFREG | 0464); st.Mode != want {
		t.Errorf("got mode %o, want %o", st.Mode, want)
	}

	if st.Size != int64(len(want)) || st.Blocks != 8 || st.Blksize != 4096 {
		t.Errorf("got %#v, want sz = %d, 8 blocks, 4096 blocksize", st, len(want))
	}

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

	replace := []byte("replaced!")
	if err := ioutil.WriteFile(mntDir+"/file", replace, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if gotBytes, err := ioutil.ReadFile(mntDir + "/file"); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if bytes.Compare(replace, gotBytes) != 0 {
		t.Fatalf("read: got %q want %q", gotBytes, replace)
	}
}

func TestDataFileLargeRead(t *testing.T) {
	root := &Inode{}

	data := make([]byte, 256*1024)
	rand.Read(data[:])
	mntDir, _, clean := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(
				ctx,
				&MemRegularFile{
					Data: data,
					Attr: fuse.Attr{
						Mode: 0464,
					},
				},
				StableAttr{})
			n.AddChild("file", ch, false)
		},
	})
	defer clean()
	got, err := ioutil.ReadFile(mntDir + "/file")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Errorf("roundtrip read had change")
	}
}

type SymlinkerRoot struct {
	Inode
}

func (s *SymlinkerRoot) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	l := &MemSymlink{
		Data: []byte(target),
	}

	ch := s.NewPersistentInode(ctx, l, StableAttr{Mode: syscall.S_IFLNK})
	return ch, 0
}

func TestDataSymlink(t *testing.T) {
	root := &SymlinkerRoot{}

	mntDir, _, clean := testMount(t, root, nil)
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
