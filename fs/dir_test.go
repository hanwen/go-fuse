// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type dirStreamErrorNode struct {
	Inode
}

var _ = (NodeReaddirer)((*dirStreamErrorNode)(nil))

func (n *dirStreamErrorNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	return &errDirStream{}, 0
}

type errDirStream struct {
	num int
}

func (ds *errDirStream) HasNext() bool {
	return ds.num < 2
}

func (ds *errDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	ds.num++
	if ds.num == 1 {
		return fuse.DirEntry{
			Mode: fuse.S_IFREG,
			Name: "first",
			Ino:  2,
		}, 0
	}
	if ds.num == 2 {
		return fuse.DirEntry{
			Mode: fuse.S_IFREG,
			Name: "last",
			Ino:  3,
		}, syscall.EKEYEXPIRED
	}

	panic("boom")
}

func (ds *errDirStream) Close() {

}

func TestDirStreamError(t *testing.T) {
	root := &dirStreamErrorNode{}

	mnt, _ := testMount(t, root, nil)

	ds, errno := NewLoopbackDirStream(mnt)
	if errno != 0 {
		t.Fatal(errno)
	}

	if !ds.HasNext() {
		t.Error("bla")
	}

	if _, errno := ds.Next(); errno != 0 {
		t.Error("blub", errno)
	}
}
