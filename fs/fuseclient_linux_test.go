// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/fuseclient"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
)

// newClientTree serves root through an in-process fuseclient, without
// a mount.
func newClientTree(t *testing.T, root InodeEmbedder, opts *Options) (*fuseclient.Tree, *rawBridge) {
	t.Helper()
	if opts == nil {
		opts = &Options{}
	}
	opts.Debug = testutil.VerboseTest()
	opts.PanicHandler = testutil.PanicHandler(t, fuse.EIO)
	raw := NewNodeFS(root, opts)
	c, err := fuseclient.New(raw, &opts.MountOptions, fuse.Notify{})
	if err != nil {
		t.Fatal(err)
	}
	tree := fuseclient.NewTree(c)
	t.Cleanup(tree.Close)
	return tree, raw.(*rawBridge)
}

// TestForgetClient is TestForget without a mount, so it does not
// need root to drop the kernel's caches.
func TestForgetClient(t *testing.T) {
	ttl := time.Hour
	tree, bridge := newClientTree(t, &allChildrenNode{depth: 2}, &Options{
		FirstAutomaticIno: 1,
		EntryTimeout:      &ttl,
	})
	for i := range 100 {
		for j := range 100 {
			n, _, _, errno := tree.Walk(nil, fmt.Sprintf("%d/%d", i, j))
			if errno != 0 {
				t.Fatalf("Walk: %v", errno)
			}
			tree.Unref(n)
		}
	}

	liveNodes := func() int {
		bridge.ids.mu.Lock()
		defer bridge.ids.mu.Unlock()
		return len(bridge.ids.nodes)
	}
	if got, want := liveNodes(), 1+100+100*100; got != want {
		t.Fatalf("got %d live nodes before forget, want %d", got, want)
	}
	tree.ForgetAll()
	if got := liveNodes(); got != 1 {
		t.Errorf("got %d live nodes after forget, want 1", got)
	}
}

func TestDefaultOwner(t *testing.T) {
	root := &Inode{}
	tree, _ := newClientTree(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			ch := root.NewPersistentInode(ctx, &MemRegularFile{Data: []byte("hello")}, StableAttr{})
			root.AddChild("file", ch, false)
		},
		UID: 42,
		GID: 43,
	})
	n, attr, _, errno := tree.Walk(nil, "file")
	if errno != 0 {
		t.Fatalf("Walk: %v", errno)
	}
	defer tree.Unref(n)
	if attr.Uid != 42 || attr.Gid != 43 {
		t.Errorf("got owner %d, %d, want 42, 43", attr.Uid, attr.Gid)
	}
	out, errno := tree.Getattr(nil, n.ID, nil)
	if errno != 0 {
		t.Fatalf("Getattr: %v", errno)
	}
	if out.Uid != 42 || out.Gid != 43 {
		t.Errorf("Getattr: got owner %d, %d, want 42, 43", out.Uid, out.Gid)
	}
}

// TestMemSymlinkSetattr checks what a symlink's Setattr refuses. The
// kernel does not send these; posixtest.SetattrSymlink covers what it
// does send.
func TestMemSymlinkSetattr(t *testing.T) {
	root := &Inode{}
	l := &MemSymlink{Attr: fuse.Attr{Mode: 0777}, Data: []byte("target")}
	tree, _ := newClientTree(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			ch := root.NewPersistentInode(ctx, l, StableAttr{Mode: syscall.S_IFLNK})
			root.AddChild("link", ch, false)
		},
	})
	n, _, _, errno := tree.Walk(nil, "link")
	if errno != 0 {
		t.Fatalf("Walk: %v", errno)
	}
	defer tree.Unref(n)

	in := fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_MODE
	in.Mode = 0555
	if _, errno := tree.Setattr(nil, n.ID, nil, &in); errno != syscall.EOPNOTSUPP {
		t.Errorf("chmod: got %v, want EOPNOTSUPP", errno)
	}
	in = fuse.SetAttrIn{}
	in.Valid = fuse.FATTR_SIZE
	if _, errno := tree.Setattr(nil, n.ID, nil, &in); errno != syscall.EINVAL {
		t.Errorf("truncate: got %v, want EINVAL", errno)
	}
	out, errno := tree.Getattr(nil, n.ID, nil)
	if errno != 0 {
		t.Fatalf("Getattr: %v", errno)
	}
	if out.Mode&07777 != 0777 {
		t.Errorf("mode changed to %o", out.Mode)
	}
}
