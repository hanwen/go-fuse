// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"fmt"
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
