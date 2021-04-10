// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is main program driver for a loopback filesystem that emulates
// windows semantics (no delete/rename on opened files.)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Release decreases the open count. The kernel doesn't wait with
// returning from close(), so if the caller is too quick to
// unlink/rename after calling close(), this may still trigger EBUSY.
// Kludge around this by sleeping for a bit before we check business.
var delay = flag.Duration("delay", 10*time.Microsecond,
	"wait this long before checking business")

// WindowsNode is a loopback FS node keeping track of open counts.
type WindowsNode struct {
	// WindowsNode inherits most functionality from LoopbackNode.
	fs.LoopbackNode

	mu        sync.Mutex
	openCount int
}

func (n *WindowsNode) increment() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.openCount++
}

func (n *WindowsNode) decrement() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.openCount--
}

var _ = (fs.NodeOpener)((*WindowsNode)(nil))

func (n *WindowsNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fh, flags, errno := n.LoopbackNode.Open(ctx, flags)
	if errno == 0 {
		n.increment()
	}
	return fh, flags, errno
}

var _ = (fs.NodeCreater)((*WindowsNode)(nil))

func (n *WindowsNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	inode, fh, flags, errno := n.LoopbackNode.Create(ctx, name, flags, mode, out)
	if errno == 0 {
		wn := inode.Operations().(*WindowsNode)
		wn.increment()
	}

	return inode, fh, flags, errno
}

var _ = (fs.NodeReleaser)((*WindowsNode)(nil))

func (n *WindowsNode) Release(ctx context.Context, f fs.FileHandle) syscall.Errno {
	n.decrement()
	if fr, ok := f.(fs.FileReleaser); ok {
		return fr.Release(ctx)
	}
	return 0
}

func isBusy(parent *fs.Inode, name string) bool {
	time.Sleep(*delay)
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

var _ = (fs.NodeRenamer)((*WindowsNode)(nil))

func (n *WindowsNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if isBusy(n.EmbeddedInode(), name) || isBusy(newParent.EmbeddedInode(), newName) {
		return syscall.EBUSY
	}
	return n.LoopbackNode.Rename(ctx, name, newParent, newName, flags)
}

func newWindowsNode(rootData *fs.LoopbackRoot, _ *fs.Inode, _ string, _ *syscall.Stat_t) fs.InodeEmbedder {
	n := &WindowsNode{
		LoopbackNode: fs.LoopbackNode{
			RootData: rootData,
		},
	}
	return n
}

func main() {
	log.SetFlags(log.Lmicroseconds)
	debug := flag.Bool("debug", false, "print debugging messages.")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Printf("usage: %s MOUNTPOINT ORIGINAL\n", path.Base(os.Args[0]))
		fmt.Printf("\noptions:\n")
		flag.PrintDefaults()
		os.Exit(2)
	}

	orig := flag.Arg(1)
	rootData := &fs.LoopbackRoot{
		NewNode: newWindowsNode,
		Path:    orig,
	}

	sec := time.Second
	opts := &fs.Options{
		AttrTimeout:  &sec,
		EntryTimeout: &sec,
	}
	opts.Debug = *debug
	opts.MountOptions.Options = append(opts.MountOptions.Options, "fsname="+orig)
	opts.MountOptions.Name = "winfs"
	opts.NullPermissions = true

	server, err := fs.Mount(flag.Arg(0), newWindowsNode(rootData, nil, "", nil), opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	fmt.Println("Mounted!")
	server.Wait()
}
