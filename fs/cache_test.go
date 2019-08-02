// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
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
	mntDir, _, clean := testMount(t, root, nil)
	defer clean()
	c1, err := ioutil.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 1: %v", err)
	}

	c2, err := ioutil.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 2: %v", err)
	}

	if bytes.Compare(c1, c2) != 0 {
		t.Errorf("keep read 2 got %q want read 1 %q", c2, c1)
	}

	if s := root.keep.Inode.NotifyContent(0, 100); s != OK {
		t.Errorf("NotifyContent: %v", s)
	}

	c3, err := ioutil.ReadFile(mntDir + "/keep")
	if err != nil {
		t.Fatalf("read keep 3: %v", err)
	}
	if bytes.Compare(c2, c3) == 0 {
		t.Errorf("keep read 3 got %q want different", c3)
	}

	nc1, err := ioutil.ReadFile(mntDir + "/nokeep")
	if err != nil {
		t.Fatalf("read keep 1: %v", err)
	}

	nc2, err := ioutil.ReadFile(mntDir + "/nokeep")
	if err != nil {
		t.Fatalf("read keep 2: %v", err)
	}

	if bytes.Compare(nc1, nc2) == 0 {
		t.Errorf("nokeep read 2 got %q want read 1 %q", c2, c1)
	}
}
