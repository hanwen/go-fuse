// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// WindowsNode emulates Windows FS semantics, which forbids deleting open files.
type WindowsNode struct {
	// WindowsNode inherits most functionality from LoopbackNode.
	fs.LoopbackNode

	mu        sync.Mutex
	openCount int
}

var _ = (fs.NodeOpener)((*WindowsNode)(nil))

func (n *WindowsNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fh, flags, errno := n.LoopbackNode.Open(ctx, flags)
	if errno == 0 {
		n.mu.Lock()
		defer n.mu.Unlock()

		n.openCount++
	}
	return fh, flags, errno
}

var _ = (fs.NodeCreater)((*WindowsNode)(nil))

func (n *WindowsNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	inode, fh, flags, errno := n.LoopbackNode.Create(ctx, name, flags, mode, out)
	if errno == 0 {
		wn := inode.Operations().(*WindowsNode)
		wn.openCount++
	}

	return inode, fh, flags, errno
}

var _ = (fs.NodeReleaser)((*WindowsNode)(nil))

// Release decreases the open count. The kernel doesn't wait with
// returning from close(), so if the caller is too quick to
// unlink/rename after calling close(), this may still trigger EBUSY.
func (n *WindowsNode) Release(ctx context.Context, f fs.FileHandle) syscall.Errno {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.openCount--
	if fr, ok := f.(fs.FileReleaser); ok {
		return fr.Release(ctx)
	}
	return 0
}

func isBusy(parent *fs.Inode, name string) bool {
	if ch := parent.GetChild(name); ch != nil {
		if wn, ok := ch.Operations().(*WindowsNode); ok {
			wn.mu.Lock()
			defer wn.mu.Unlock()
			if wn.openCount > 0 {
				return true
			}
		}
	}
	return false
}

var _ = (fs.NodeUnlinker)((*WindowsNode)(nil))

func (n *WindowsNode) Unlink(ctx context.Context, name string) syscall.Errno {
	if isBusy(n.EmbeddedInode(), name) {
		return syscall.EBUSY
	}

	return n.LoopbackNode.Unlink(ctx, name)
}

func newWindowsNode(rootData *fs.LoopbackRoot, parent *fs.Inode, name string, st *syscall.Stat_t) fs.InodeEmbedder {
	n := &WindowsNode{
		LoopbackNode: fs.LoopbackNode{
			RootData: rootData,
		},
	}
	return n
}

// ExampleLoopbackReuse shows how to build a file system on top of the
// loopback file system.
func Example_loopbackReuse() {
	mntDir := "/tmp/mnt"
	origDir := "/tmp/orig"

	rootData := &fs.LoopbackRoot{
		NewNode: newWindowsNode,
		Path:    origDir,
	}

	sec := time.Second
	opts := &fs.Options{
		AttrTimeout:  &sec,
		EntryTimeout: &sec,
	}

	server, err := fs.Mount(mntDir, newWindowsNode(rootData, nil, "", nil), opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	fmt.Printf("files under %s cannot be deleted if they are opened", mntDir)
	server.Wait()
}
