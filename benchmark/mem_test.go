// Copyright 2025 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type memFile struct {
	fs.MemRegularFile
}

var _ = (fs.NodeGetxattrer)((*memFile)(nil))
var _ = (fs.NodeFsyncer)((*memFile)(nil))

func (mf *memFile) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	// Suppress security attribute reads
	return 0, syscall.ENOSYS
}

func (mf *memFile) Fsync(context.Context, fs.FileHandle, uint32) syscall.Errno {
	return 0
}

type memDir struct {
	fs.Inode
}

func (md *memDir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	mrf := memFile{}
	ch := md.NewInode(ctx, &mrf, fs.StableAttr{Mode: fuse.S_IFREG})
	md.AddChild(name, ch, true)

	return ch, nil, 0, 0
}

func TestBenchmarkMemFSFio(t *testing.T) {
	root := &memDir{}
	mnt := t.TempDir()

	opts := fs.Options{}
	opts.Debug = false //  logging impacts performance.
	ttl := 100 * time.Second
	opts.EntryTimeout = &ttl
	opts.AttrTimeout = &ttl
	srv, err := fs.Mount(mnt, root, &opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.Unmount()
	})
	srv.WaitMount()

	mode := "read"
	cmd := exec.Command("fio", "--directory="+mnt,
		"--rw="+mode, "--name="+mode,
		"--bs=128k", "--size=1G", "--direct=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
