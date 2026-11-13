// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
)

func TestMemFileExtendingFallocate(t *testing.T) {
	// len < cap, with stale bytes beyond len.
	backing := []byte("hello, stale bytes")
	data := backing[:5]

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
						Mode: 0644,
					},
				},
				StableAttr{})
			n.AddChild("file", ch, false)
		},
	})

	fn := mntDir + "/file"
	fd, err := syscall.Open(fn, syscall.O_RDWR, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer syscall.Close(fd)

	for _, sz := range []int{10, 2 * len(backing)} {
		if err := syscall.Fallocate(fd, 0, 0, int64(sz)); err != nil {
			t.Fatalf("Fallocate(%d): %v", sz, err)
		}
		got, err := os.ReadFile(fn)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		want := append([]byte("hello"), make([]byte, sz-5)...)
		if !bytes.Equal(got, want) {
			t.Errorf("fallocate to %d: got %q, want %q", sz, got, want)
		}
	}
}

func TestMemFileReadPastEOF(t *testing.T) {
	pgsz := os.Getpagesize()
	mrf := &MemRegularFile{
		Data: bytes.Repeat([]byte{'x'}, 2*pgsz),
		Attr: fuse.Attr{
			Mode: 0644,
		},
	}

	root := &Inode{}
	ttl := time.Hour
	mntDir, _ := testMount(t, root, &Options{
		FirstAutomaticIno: 1,
		AttrTimeout:       &ttl,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(ctx, mrf, StableAttr{})
			n.AddChild("file", ch, false)
		},
	})

	fn := mntDir + "/file"

	// Open before shrinking: the LOOKUP at open time caches the
	// original size, and with the fd held no later lookup can refresh
	// it. O_DIRECT bypasses the page cache, so the read below reaches
	// MemRegularFile.Read at an offset past the real EOF instead of
	// being served (or clamped) by the kernel.
	fd, err := syscall.Open(fn, syscall.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		t.Fatalf("Open(O_DIRECT): %v", err)
	}
	defer syscall.Close(fd)

	// Shrink the file behind the kernel's back; the cached size stays
	// at 2*pgsz for the duration of AttrTimeout.
	mrf.mu.Lock()
	mrf.Data = mrf.Data[:1]
	mrf.mu.Unlock()

	buf := make([]byte, pgsz)
	n, err := syscall.Pread(fd, buf, int64(pgsz))
	if err != nil {
		t.Fatalf("Pread past EOF: %v", err)
	}
	if n != 0 {
		t.Errorf("Pread past EOF returned %d bytes, want 0", n)
	}
}
