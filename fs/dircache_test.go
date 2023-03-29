// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type dirCacheTestNode struct {
	Inode
}

var _ = (NodeOpendirer)((*dirCacheTestNode)(nil))

func (n *dirCacheTestNode) Opendir(ctx context.Context) (uint32, syscall.Errno) {
	return fuse.FOPEN_CACHE_DIR, 0
}

var _ = (NodeReaddirer)((*dirCacheTestNode)(nil))

func (n *dirCacheTestNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	var entries []fuse.DirEntry

	for i := 0; i < 1024; i++ {
		entries = append(entries, fuse.DirEntry{
			Name: fmt.Sprintf("file%04d", i),
			Ino:  uint64(i + 42),
			Mode: fuse.S_IFREG,
		})
	}
	return NewListDirStream(entries), 0
}

func TestDirCacheFlag(t *testing.T) {
	root := &dirCacheTestNode{}
	opts := Options{}
	opts.DisableReadDirPlus = true
	mnt, _, clean := testMount(t, root, &opts)
	defer clean()

	for i := 0; i < 2; i++ {
		s, errno := NewLoopbackDirStream(mnt)
		if errno != 0 {
			t.Fatalf("NewLoopbackDirStream: %v", errno)
		}

		for s.HasNext() {
			s.Next()
		}
		s.Close()
	}
}
