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
		}, syscall.EBADMSG
	}

	panic("boom")
}

func (ds *errDirStream) Close() {

}

func TestDirStreamError(t *testing.T) {
	for _, disableReaddirplus := range []bool{false, true} {
		t.Run(fmt.Sprintf("disableReaddirplus=%v", disableReaddirplus),
			func(t *testing.T) {
				root := &dirStreamErrorNode{}
				opts := Options{}
				opts.DisableReadDirPlus = disableReaddirplus

				mnt, _ := testMount(t, root, &opts)

				ds, errno := NewLoopbackDirStream(mnt)
				if errno != 0 {
					t.Fatalf("NewLoopbackDirStream: %v", errno)
				}
				defer ds.Close()

				if e, errno := ds.Next(); errno != 0 {
					t.Errorf("ds.Next: %v", errno)
				} else if e.Name != "first" {
					t.Errorf("got %q want 'first'", e.Name)
				}
				// Here we need choose a errno to test if errno could be passed and handled
				// correctly by the fuse library. To build the test on different platform,
				// an errno which defined on each platform should be chosen. And if the
				// chosen integer number is not a valid errno, the fuse in kernel would refuse
				// and throw an error, which is observed on Linux.
				// Here we choose to use EBADMSG, which is defined on multiple Unix-like OSes.
				if _, errno := ds.Next(); errno != syscall.EBADMSG {
					t.Errorf("got errno %v, want EBADMSG", errno)
				}
			})
	}
}
