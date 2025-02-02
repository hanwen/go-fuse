// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sys/unix"
)

type keepCacheFile struct {
	Inode
	keepCache bool

	mu      sync.Mutex
	content []byte
	count   int
}

var _ = (NodeReader)((*keepCacheFile)(nil))
var _ = (NodeOpener)((*keepCacheFile)(nil))
var _ = (NodeGetattrer)((*keepCacheFile)(nil))

func (f *keepCacheFile) setContent(delta int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count += delta
	f.content = []byte(fmt.Sprintf("%010x", f.count))
}

func (f *keepCacheFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	var fl uint32
	if f.keepCache {
		fl = fuse.FOPEN_KEEP_CACHE
	}

	f.setContent(0)
	return nil, fl, OK
}

func (f *keepCacheFile) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	out.Size = uint64(len(f.content))

	return OK
}

func (f *keepCacheFile) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	f.setContent(1)

	f.mu.Lock()
	defer f.mu.Unlock()

	return fuse.ReadResultData(f.content[off:]), OK
}

type keepCacheRoot struct {
	Inode

	keep, nokeep *keepCacheFile
}

var _ = (NodeOnAdder)((*keepCacheRoot)(nil))

func (r *keepCacheRoot) OnAdd(ctx context.Context) {
	i := &r.Inode

	r.keep = &keepCacheFile{
		keepCache: true,
	}
	r.keep.setContent(0)
	i.AddChild("keep", i.NewInode(ctx, r.keep, StableAttr{}), true)

	r.nokeep = &keepCacheFile{
		keepCache: false,
	}
	r.nokeep.setContent(0)
	i.AddChild("nokeep", i.NewInode(ctx, r.nokeep, StableAttr{}), true)
}

// Test FOPEN_KEEP_CACHE. This is a little subtle: the automatic cache
// invalidation triggers if mtime or file size is changed, so only
// change content but no metadata.
func TestKeepCache(t *testing.T) {
	root := &keepCacheRoot{}
	mntDir, _ := testMount(t, root, nil)

	c1, err := os.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 1: %v", err)
	}

	c2, err := os.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 2: %v", err)
	}

	if bytes.Compare(c1, c2) != 0 {
		t.Errorf("keep read 2 got %q want read 1 %q", c2, c1)
	}

	if s := root.keep.Inode.NotifyContent(0, 100); s != OK {
		t.Errorf("NotifyContent: %v", s)
	}

	c3, err := os.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 3: %v", err)
	}
	if bytes.Compare(c2, c3) == 0 {
		t.Errorf("keep read 3 got %q want different", c3)
	}

	nc1, err := os.ReadFile(mntDir + "/nokeep")
	if err != nil {
		t.Fatalf("read keep 1: %v", err)
	}

	nc2, err := os.ReadFile(mntDir + "/nokeep")
	if err != nil {
		t.Fatalf("read keep 2: %v", err)
	}

	if bytes.Compare(nc1, nc2) == 0 {
		t.Errorf("nokeep read 2 got %q want read 1 %q", c2, c1)
	}
}

type countingSymlink struct {
	Inode

	mu        sync.Mutex
	readCount int
	data      []byte
}

var _ = (NodeGetattrer)((*countingSymlink)(nil))

func (l *countingSymlink) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	l.mu.Lock()
	defer l.mu.Unlock()
	out.Attr.Size = uint64(len(l.data))
	return 0
}

var _ = (NodeReadlinker)((*countingSymlink)(nil))

func (l *countingSymlink) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.readCount++
	return l.data, 0
}

func (l *countingSymlink) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.readCount
}

func TestSymlinkCaching(t *testing.T) {
	mnt := t.TempDir()
	want := "target"
	link := countingSymlink{
		data: []byte(want),
	}
	sz := len(link.data)
	root := &Inode{}
	dt := 10 * time.Millisecond
	opts := &Options{
		EntryTimeout: &dt,
		AttrTimeout:  &dt,
		OnAdd: func(ctx context.Context) {
			root.AddChild("link",
				root.NewPersistentInode(ctx, &link, StableAttr{Mode: syscall.S_IFLNK}), false)
		},
	}
	opts.Debug = testutil.VerboseTest()
	opts.EnableSymlinkCaching = true

	server, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	for i := 0; i < 2; i++ {
		if got, err := os.Readlink(mnt + "/link"); err != nil {
			t.Fatal(err)
		} else if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}

	if c := link.count(); c != 1 {
		t.Errorf("got %d want 1", c)
	}

	if errno := link.NotifyContent(0, int64(sz)); errno != 0 {
		t.Fatalf("NotifyContent: %v", errno)
	}
	if _, err := os.Readlink(mnt + "/link"); err != nil {
		t.Fatal(err)
	}

	if c := link.count(); c != 2 {
		t.Errorf("got %d want 2", c)
	}

	// The actual test goes till here. The below is just to
	// clarify behavior of the feature: changed attributes do not
	// trigger reread, and the Attr.Size is used to truncate a
	// previous read result.
	link.mu.Lock()
	link.data = []byte("x")
	link.mu.Unlock()

	time.Sleep((3 * dt) / 2)
	if l, err := os.Readlink(mnt + "/link"); err != nil {
		t.Fatal(err)
	} else if l != want[:1] {
		log.Printf("got %q want %q", l, want[:1])
	}
	if c := link.count(); c != 2 {
		t.Errorf("got %d want 2", c)
	}
}

type mmapTestNode struct {
	Inode

	mu      sync.Mutex
	content []byte
	mtime   time.Time
}

var _ = (NodeOpener)((*mmapTestNode)(nil))

func (f *mmapTestNode) Open(ctx context.Context, openFlags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, 0, 0
}

var _ = (NodeReader)((*mmapTestNode)(nil))

func (f *mmapTestNode) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	return fuse.ReadResultData(f.content[off : int(off)+len(dest)]), OK
}

var _ = (NodeGetattrer)((*mmapTestNode)(nil))

func (f *mmapTestNode) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = uint64(len(f.content))
	out.SetTimes(nil, &f.mtime, nil)
	return OK
}

/* Exercise mmap() and AUTO_INVAL_DATA */
func TestMMap(t *testing.T) {
	mnt := t.TempDir()

	node := mmapTestNode{
		content: bytes.Repeat([]byte{'x'}, 4096),
		mtime:   time.Now(),
	}
	root := &Inode{}
	dt := 10 * time.Millisecond
	opts := &Options{
		EntryTimeout: &dt,
		AttrTimeout:  &dt,
		OnAdd: func(ctx context.Context) {
			root.AddChild("file",
				root.NewPersistentInode(ctx, &node, StableAttr{Mode: syscall.S_IFREG}), false)
		},
	}
	opts.Debug = testutil.VerboseTest()

	srv, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Unmount()
	f, err := os.Open(mnt + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := unix.Mmap(int(f.Fd()), 0, int(8<<10), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Munmap(data)
	if data[1] != 'x' {
		t.Errorf("got %c want x", data[1])
	}

	node.mu.Lock()
	node.mtime = node.mtime.Add(time.Hour)
	node.content = bytes.Repeat([]byte{'y'}, 4096)
	node.mu.Unlock()

	if _, err := f.Stat(); err != nil {
		t.Fatal(err)
	}

	if data[1] != 'y' {
		t.Errorf("got %c want y", data[1])
	}
}
