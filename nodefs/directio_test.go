// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type dioRoot struct {
	DefaultOperations
}

func (r *dioRoot) OnAdd(ctx context.Context) {
	r.Inode().AddChild("file", r.Inode().NewInode(ctx, &dioFile{}, NodeAttr{}), false)
}

// A file handle that pretends that every hole/data starts at
// multiples of 1024
type dioFH struct {
	DefaultFileHandle
}

func (f *dioFH) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, fuse.Status) {
	next := (off + 1023) & (^uint64(1023))
	return next, fuse.OK
}

func (fh *dioFH) Read(ctx context.Context, data []byte, off int64) (fuse.ReadResult, fuse.Status) {
	r := bytes.Repeat([]byte(fmt.Sprintf("%010d", off)), 1+len(data)/10)
	return fuse.ReadResultData(r[:len(data)]), fuse.OK
}

// overrides Open so it can return a dioFH file handle
type dioFile struct {
	DefaultOperations
}

func (f *dioFile) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, status fuse.Status) {
	return &dioFH{}, fuse.FOPEN_DIRECT_IO, fuse.OK
}

func TestDirectIO(t *testing.T) {
	root := &dioRoot{}
	mntDir := testutil.TempDir()

	defer os.RemoveAll(mntDir)
	server, err := Mount(mntDir, root, &Options{
		MountOptions: fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		},
		FirstAutomaticIno: 1,

		// no caching.
	})
	defer server.Unmount()

	f, err := os.Open(mntDir + "/file")
	if err != nil {
		t.Fatalf("Open %v", err)
	}
	defer f.Close()

	var buf [10]byte
	n, err := f.Read(buf[:])
	if err != nil {
		t.Fatalf("Read %v", err)
	}
	want := bytes.Repeat([]byte{'0'}, 10)
	got := buf[:n]
	if bytes.Compare(got, want) != 0 {
		t.Errorf("got %q want %q", got, want)
	}

	const SEEK_DATA = 3 /* seek to the next data */

	if n, err := syscall.Seek(int(f.Fd()), 512, SEEK_DATA); err != nil {
		t.Errorf("Seek: %v", err)
	} else if n != 1024 {
		t.Errorf("seek: got %d, want %d", n, 1024)
	}

	n, err = f.Read(buf[:])
	if err != nil {
		t.Fatalf("Read %v", err)
	}
	want = []byte(fmt.Sprintf("%010d", 1024))
	got = buf[:n]
	if bytes.Compare(got, want) != 0 {
		t.Errorf("got %q want %q", got, want)
	}
}
