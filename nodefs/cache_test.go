// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type keepCacheFile struct {
	DefaultOperations
	keepCache bool

	mu      sync.Mutex
	content []byte
	count   int
}

func (f *keepCacheFile) setContent(delta int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count += delta
	f.content = []byte(fmt.Sprintf("%010x", f.count))
}

func (f *keepCacheFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, fuse.Status) {
	var fl uint32
	if f.keepCache {
		fl = fuse.FOPEN_KEEP_CACHE
	}

	f.setContent(0)
	return nil, fl, fuse.OK
}

func (f *keepCacheFile) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	out.Size = uint64(len(f.content))

	return fuse.OK
}

func (f *keepCacheFile) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, fuse.Status) {
	f.setContent(1)

	f.mu.Lock()
	defer f.mu.Unlock()

	return fuse.ReadResultData(f.content[off:]), fuse.OK
}

type keepCacheRoot struct {
	DefaultOperations

	keep, nokeep *keepCacheFile
}

func (r *keepCacheRoot) OnAdd(ctx context.Context) {
	i := InodeOf(r)

	r.keep = &keepCacheFile{
		keepCache: true,
	}
	r.keep.setContent(0)
	i.AddChild("keep", i.NewInode(ctx, r.keep, NodeAttr{}), true)

	r.nokeep = &keepCacheFile{
		keepCache: false,
	}
	r.nokeep.setContent(0)
	i.AddChild("nokeep", i.NewInode(ctx, r.nokeep, NodeAttr{}), true)
}

func TestKeepCache(t *testing.T) {
	mntDir := testutil.TempDir()
	sec := time.Second
	root := &keepCacheRoot{}
	rawFS := NewNodeFS(root, &Options{
		Debug:             testutil.VerboseTest(),
		FirstAutomaticIno: 1,

		AttrTimeout:  &sec,
		EntryTimeout: &sec,
	})
	server, err := fuse.NewServer(rawFS, mntDir,
		&fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		})

	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}

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

	if s := InodeOf(root.keep).NotifyContent(0, 100); !s.Ok() {
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
