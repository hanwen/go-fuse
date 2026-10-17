// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type SimpleFS struct {
	Inode
}

func (fs *SimpleFS) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	const numOfEntries = 16

	entries := make([]fuse.DirEntry, 0, numOfEntries)
	for i := 0; i < numOfEntries; i++ {
		entries = append(entries, fuse.DirEntry{
			Name: fmt.Sprintf("name%04d", i),
			Mode: fuse.S_IFREG,
			Ino:  uint64(i + 100),
		})
	}
	return NewListDirStream(entries), 0
}

func TestDirSeek(t *testing.T) {
	mountDir, server := testMount(t, &SimpleFS{}, nil)
	defer os.RemoveAll(mountDir)
	defer server.Unmount()

	testDirSeek(t, mountDir)
}

type rewindDirNode struct {
	Inode
	mu      sync.Mutex
	entries []string
}

var _ = (NodeReaddirer)((*rewindDirNode)(nil))

func (n *rewindDirNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	list := make([]fuse.DirEntry, len(n.entries))
	for i, name := range n.entries {
		list[i] = fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFREG,
			Ino:  uint64(i + 100),
		}
	}
	return NewListDirStream(list), 0
}

func (n *rewindDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, entry := range n.entries {
		if entry == name {
			stable := StableAttr{
				Mode: fuse.S_IFREG | 0644,
				Ino:  uint64(i + 100),
			}
			return n.NewPersistentInode(ctx, &MemRegularFile{}, stable), 0
		}
	}
	return nil, syscall.ENOENT
}

// TestDirRewind checks that rewinddir on a NodeReaddirer directory
// refreshes the listing instead of replaying the old DirStream.
func TestDirRewind(t *testing.T) {
	root := &rewindDirNode{entries: []string{"a"}}
	mntDir, _ := testMount(t, root, nil)

	f, err := os.Open(mntDir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if names, err := f.Readdirnames(-1); err != nil || len(names) != 1 || names[0] != "a" {
		t.Fatalf("got %v, %v, want [a]", names, err)
	}

	root.mu.Lock()
	root.entries = append(root.entries, "b")
	root.mu.Unlock()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("after rewind: got %v, want [a b]", names)
	}
}

type rewindHandleNode struct {
	Inode
	mu      sync.Mutex
	entries []string
}

var _ = (NodeOpendirHandler)((*rewindHandleNode)(nil))

func (n *rewindHandleNode) OpendirHandle(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	h := &rewindHandle{node: n}
	h.Seekdir(ctx, 0)
	return h, 0, 0
}

type rewindHandle struct {
	node    *rewindHandleNode
	entries []string
	idx     int
}

var _ = (FileReaddirenter)((*rewindHandle)(nil))
var _ = (FileSeekdirer)((*rewindHandle)(nil))

func (h *rewindHandle) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	if off == 0 {
		h.node.mu.Lock()
		h.entries = append([]string{}, h.node.entries...)
		h.node.mu.Unlock()
	}
	if off > uint64(len(h.entries)) {
		return syscall.EINVAL
	}
	h.idx = int(off)
	return 0
}

func (h *rewindHandle) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if h.idx >= len(h.entries) {
		return nil, 0
	}
	h.idx++
	return &fuse.DirEntry{
		Name: h.entries[h.idx-1],
		Mode: fuse.S_IFREG,
		Ino:  uint64(h.idx + 100),
		Off:  uint64(h.idx),
	}, 0
}

// TestDirRewindEmpty checks that rewinddir on a handle that was read
// while empty reaches FileSeekdirer.Seekdir(0).
func TestDirRewindEmpty(t *testing.T) {
	root := &rewindHandleNode{}
	mntDir, _ := testMount(t, root, nil)

	f, err := os.Open(mntDir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if names, err := f.Readdirnames(-1); err != nil || len(names) != 0 {
		t.Fatalf("got %v, %v, want empty", names, err)
	}

	root.mu.Lock()
	root.entries = append(root.entries, "a")
	root.mu.Unlock()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "a" {
		t.Errorf("after rewind: got %v, want [a]", names)
	}
}
