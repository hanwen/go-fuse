// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type treeAddTestNode struct {
	Inode

	mu     sync.Mutex
	parent *Inode
}

type treeAddTestRoot struct {
	Inode

	child treeAddTestNode
}

var _ = (NodeLookuper)((*treeAddTestRoot)(nil))

func (r *treeAddTestRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	ch := r.NewInode(ctx, &r.child, StableAttr{Mode: fuse.S_IFREG})
	return ch, 0
}

func (n *treeAddTestNode) OnTreeAdd(ctx context.Context) {
	n.mu.Lock()
	defer n.mu.Unlock()
	_, n.parent = n.Parent()
}

func TestOnTreeAdd(t *testing.T) {
	root := treeAddTestRoot{}
	mntDir, _ := testMount(t, &root, &Options{})

	_, err := os.Stat(mntDir + "/file")
	if err != nil {
		t.Fatal(err)
	}

	root.child.mu.Lock()
	defer root.child.mu.Unlock()
	if root.child.parent != &root.Inode {
		t.Fatalf("should have a parent, got %v", root.child.parent)
	}
}
