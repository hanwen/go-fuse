// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type dioRoot struct {
	Inode
}

func (r *dioRoot) OnAdd(ctx context.Context) {
	r.Inode.AddChild("file", r.Inode.NewInode(ctx, &dioFile{}, StableAttr{}), false)
}

// A file handle that pretends that every hole/data starts at
// multiples of 1024
type dioFH struct {
}

var _ = (FileLseeker)((*dioFH)(nil))
var _ = (FileReader)((*dioFH)(nil))

func (f *dioFH) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	next := (off + 1023) & (^uint64(1023))
	return next, OK
}

func (fh *dioFH) Read(ctx context.Context, data []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	r := bytes.Repeat([]byte(fmt.Sprintf("%010d", off)), 1+len(data)/10)
	return fuse.ReadResultData(r[:len(data)]), OK
}

// overrides Open so it can return a dioFH file handle
type dioFile struct {
	Inode
}

var _ = (NodeOpener)((*dioFile)(nil))

func (f *dioFile) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return &dioFH{}, fuse.FOPEN_DIRECT_IO, OK
}

// this tests FOPEN_DIRECT_IO (as opposed to O_DIRECTIO)
func TestFUSEDirectIO(t *testing.T) {
	root := &dioRoot{}
	mntDir, server, clean := testMount(t, root, nil)
	defer clean()

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

	if !server.KernelSettings().SupportsVersion(7, 24) {
		t.Skip("Kernel does not support lseek")
	}
	if n, err := syscall.Seek(int(f.Fd()), 512, _SEEK_DATA); err != nil {
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
