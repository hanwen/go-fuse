// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package nfs holds the NFS export reconnect test. It is a separate
// package (rather than fs_test) so its go-nfs-client dependency does
// not leak into the main module's test binary.
package nfs

import (
	"context"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type synthEntry struct {
	mode     uint32 // S_IFDIR or S_IFREG
	data     []byte // file content, for S_IFREG entries
	children map[string]uint64
}

// synthTree is the shared backing store behind every mount of the test
// filesystem: root=1, "a"=2, "a/b"=3, "a/b/file.txt"=4. Two directory
// levels ensure that ".." resolution passes through a non-root parent.
type synthTree struct {
	mu      sync.Mutex
	entries map[uint64]*synthEntry
}

func newSynthTree(fileData []byte) *synthTree {
	return &synthTree{
		entries: map[uint64]*synthEntry{
			1: {mode: syscall.S_IFDIR, children: map[string]uint64{"a": 2}},
			2: {mode: syscall.S_IFDIR, children: map[string]uint64{"b": 3}},
			3: {mode: syscall.S_IFDIR, children: map[string]uint64{"file.txt": 4}},
			4: {mode: syscall.S_IFREG, data: fileData},
		},
	}
}

type synthNode struct {
	fs.Inode
	tree *synthTree
	ino  uint64
}

var _ = (fs.NodeLookuper)((*synthNode)(nil))

func (n *synthNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.tree.mu.Lock()
	e := n.tree.entries[n.ino]
	childIno, ok := e.children[name]
	n.tree.mu.Unlock()
	if !ok {
		return nil, syscall.ENOENT
	}
	return n.newChild(ctx, childIno, out), 0
}

var _ = (fs.NodeLookupNoder)((*synthNode)(nil))

func (n *synthNode) LookupNode(ctx context.Context, id uint64, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.tree.mu.Lock()
	_, ok := n.tree.entries[id]
	n.tree.mu.Unlock()
	if !ok {
		return nil, syscall.ENOENT
	}
	return n.newChild(ctx, id, out), 0
}

var _ = (fs.NodeLookupParenter)((*synthNode)(nil))

// LookupParent scans for the entry listing n.ino as a child; a real
// filesystem would track parents directly.
func (n *synthNode) LookupParent(ctx context.Context, out *fuse.EntryOut) (*fs.Inode, string, syscall.Errno) {
	n.tree.mu.Lock()
	defer n.tree.mu.Unlock()
	for parentIno, e := range n.tree.entries {
		for name, childIno := range e.children {
			if childIno == n.ino {
				fillAttr(&out.Attr, n.tree.entries[parentIno])
				ops := &synthNode{tree: n.tree, ino: parentIno}
				parent := n.NewInode(ctx, ops, fs.StableAttr{Mode: n.tree.entries[parentIno].mode, Ino: parentIno})
				return parent, name, 0
			}
		}
	}
	return nil, "", syscall.ENOENT
}

func (n *synthNode) newChild(ctx context.Context, ino uint64, out *fuse.EntryOut) *fs.Inode {
	n.tree.mu.Lock()
	e := n.tree.entries[ino]
	fillAttr(&out.Attr, e)
	n.tree.mu.Unlock()

	ops := &synthNode{tree: n.tree, ino: ino}
	return n.NewInode(ctx, ops, fs.StableAttr{Mode: e.mode, Ino: ino})
}

var _ = (fs.NodeGetattrer)((*synthNode)(nil))

func (n *synthNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.tree.mu.Lock()
	defer n.tree.mu.Unlock()
	fillAttr(&out.Attr, n.tree.entries[n.ino])
	return 0
}

func fillAttr(out *fuse.Attr, e *synthEntry) {
	out.Mode = e.mode
	if e.mode&syscall.S_IFDIR == 0 {
		out.Size = uint64(len(e.data))
	}
}

var _ = (fs.NodeOpener)((*synthNode)(nil))

func (n *synthNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return nil, 0, 0
}

var _ = (fs.NodeReader)((*synthNode)(nil))

func (n *synthNode) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n.tree.mu.Lock()
	data := n.tree.entries[n.ino].data
	n.tree.mu.Unlock()

	off = min(off, int64(len(data)))
	end := min(int(off)+len(dest), len(data))
	return fuse.ReadResultData(data[off:end]), 0
}

func mount(dir string, tree *synthTree) (*fuse.Server, error) {
	root := &synthNode{tree: tree, ino: 1}
	return fs.Mount(dir, root, &fs.Options{
		ExternalNodeID: true,
		RootStableAttr: &fs.StableAttr{Ino: 1},
		MountOptions: fuse.MountOptions{
			ExtraCapabilities: fuse.CAP_EXPORT_SUPPORT,
			Debug:             testutil.VerboseTest(),
		},
	})
}
