// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
)

func testMount(t *testing.T, root InodeEmbedder, opts *Options) (string, *fuse.Server) {
	t.Helper()

	mntDir := t.TempDir()
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
	t.Cleanup(func() {
		if err := server.Unmount(); err != nil {
			t.Fatalf("testMount: Unmount failed: %v", err)
		}
	})
	return mntDir, server
}

func TestDefaultOwner(t *testing.T) {
	want := "hello"
	root := &Inode{}
	mntDir, _ := testMount(t, root, &Options{
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

	var st syscall.Stat_t
	if err := syscall.Lstat(mntDir+"/file", &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	} else if st.Uid != 42 || st.Gid != 43 {
		t.Fatalf("Got Lstat %d, %d want 42,43", st.Uid, st.Gid)
	}
}

func TestRootInode(t *testing.T) {
	var rootIno uint64 = 42
	root := &Inode{}

	mntDir, _ := testMount(t, root, &Options{
		RootStableAttr: &StableAttr{
			Ino: rootIno,
			Gen: 1,
		},
	})

	var st syscall.Stat_t
	if err := syscall.Lstat(mntDir, &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	} else if st.Ino != rootIno {
		t.Fatalf("Got Lstat inode %d, want %d", st.Ino, rootIno)
	}
}

func TestLseekDefault(t *testing.T) {
	data := []byte("hello")
	root := &Inode{}
	mntDir, _ := testMount(t, root, &Options{
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
				}, StableAttr{})
			n.AddChild("file.bin", ch, false)
		},
	})

	posixtest.LseekHoleSeeksToEOF(t, mntDir)
}

func TestDataFile(t *testing.T) {
	want := "hello"
	root := &Inode{}
	mntDir, _ := testMount(t, root, &Options{
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

	var st syscall.Stat_t
	if err := syscall.Lstat(mntDir+"/file", &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if want := uint(syscall.S_IFREG | 0464); uint(st.Mode) != want {
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
	mntDir, _ := testMount(t, root, &Options{
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

	mntDir, _ := testMount(t, root, nil)

	if err := syscall.Symlink("target", mntDir+"/link"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if got, err := os.Readlink(mntDir + "/link"); err != nil {
		t.Fatalf("Readlink: %v", err)
	} else if want := "target"; got != want {
		t.Errorf("Readlink: got %q want %q", got, want)
	}
}

func TestReaddirplusParallel(t *testing.T) {
	root := &Inode{}
	N := 100
	oneSec := time.Second
	names := map[string]int64{}
	mntDir, _ := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		EntryTimeout:      &oneSec,
		AttrTimeout:       &oneSec,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()

			for i := 0; i < N; i++ {
				ch := n.NewPersistentInode(
					ctx,
					&MemRegularFile{
						Data: bytes.Repeat([]byte{'x'}, i),
					},
					StableAttr{})

				name := fmt.Sprintf("file%04d", i)
				names[name] = int64(i)
				n.AddChild(name, ch, false)
			}
		},
	})

	read := func() (map[string]int64, error) {
		es, err := os.ReadDir(mntDir)
		if err != nil {
			return nil, err
		}

		r := map[string]int64{}
		for _, e := range es {
			inf, err := e.Info()
			if err != nil {
				return nil, err
			}
			r[e.Name()] = inf.Size()
		}
		return r, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := read()
			if err != nil {
				t.Errorf("readdir: %v", err)
			}
			if got, want := len(res), len(names); got != want {
				t.Errorf("got %d want %d", got, want)
				return
			}
			if !reflect.DeepEqual(res, names) {
				t.Errorf("maps have different content")
			}
		}()
	}
	wg.Wait()
}
