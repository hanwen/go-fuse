// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type rootNode struct {
	nodefs.Node
	// represents backing store.
	mu      sync.Mutex
	backing map[string]string
}

type blobNode struct {
	nodefs.Node
	content string
}

func (n *blobNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) (code fuse.Status) {
	out.Mode = fuse.S_IFREG | 0777
	out.Size = uint64(len(n.content))
	return fuse.OK
}

func (n *rootNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	n.mu.Lock()
	defer n.mu.Unlock()
	want := n.backing[name]

	if want == "" {
		return nil, fuse.ENOENT
	}

	ch := n.Inode().GetChild(name)
	var blob *blobNode
	if ch != nil {
		blob = ch.Node().(*blobNode)
		if blob.content != want {
			n.Inode().RmChild(name)
			ch = nil
		}
	}

	if ch == nil {
		blob = &blobNode{nodefs.NewDefaultNode(), want}
		ch = n.Inode().NewChild(name, false, blob)
	}

	status := blob.GetAttr(out, nil, nil)
	return ch, status
}

func TestUpdateNode(t *testing.T) {
	dir := testutil.TempDir()
	root := &rootNode{
		Node:    nodefs.NewDefaultNode(),
		backing: map[string]string{"a": "aaa"},
	}

	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	opts.EntryTimeout = 0
	opts.LookupKnownChildren = true

	server, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	go server.Serve()

	if err := server.WaitMount(); err != nil {
		t.Fatalf("WaitMount: %v", err)
	}

	defer server.Unmount()

	fi1, err := os.Lstat(dir + "/a")
	if err != nil {
		t.Fatal("Lstat", err)
	}

	if fi1.Size() != 3 {
		t.Fatalf("got %v, want sz 3", fi1.Size())
	}

	root.mu.Lock()
	root.backing["a"] = "x"
	root.mu.Unlock()

	fi2, err := os.Lstat(dir + "/a")
	if err != nil {
		t.Fatal("Lstat", err)
	}

	if fi2.Size() != 1 {
		t.Fatalf("got %#v, want sz 1", fi2)
	}
}
