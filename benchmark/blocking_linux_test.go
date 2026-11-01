// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// blockingFS has a file for every name. Getattr on a file blocks in a
// syscall for the given time, like a file system that reads its data
// from disk.
type blockingFS struct {
	fs.Inode
	block time.Duration
}

type blockingFile struct {
	fs.Inode
	block time.Duration
}

var _ = (fs.NodeLookuper)((*blockingFS)(nil))

func (n *blockingFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return n.NewInode(ctx, &blockingFile{block: n.block}, fs.StableAttr{Mode: fuse.S_IFREG}), 0
}

var _ = (fs.NodeGetattrer)((*blockingFile)(nil))

func (n *blockingFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if n.block > 0 {
		// Not time.Sleep: that parks the goroutine, but a
		// blocking syscall keeps its thread and P.
		ts := syscall.NsecToTimespec(n.block.Nanoseconds())
		syscall.Nanosleep(&ts, nil)
	}
	out.Mode = fuse.S_IFREG | 0o644
	return 0
}

// BenchmarkBlockingGetAttr stats files from 4 threads in another
// process, while Getattr blocks in a syscall. With few Ps, all of them
// can then be in a syscall when a request arrives, so the server
// depends on how a waiting reader gets a P.
func BenchmarkBlockingGetAttr(b *testing.B) {
	for _, block := range []time.Duration{0, 20 * time.Microsecond, 200 * time.Microsecond} {
		b.Run(fmt.Sprintf("block=%v", block), func(b *testing.B) {
			hour := time.Hour
			zero := time.Duration(0)
			mnt := setupFSWithOptions(&blockingFS{block: block}, &fs.Options{
				EntryTimeout: &hour,
				AttrTimeout:  &zero,
			}, b)
			const threads = 4
			// Look up the files, so the timed stats only GETATTR.
			runBulkstat(b, "-mode=getattr", threads, threads, mnt)
			b.ReportAllocs()
			runBulkstat(b, "-mode=getattr", threads, b.N, mnt)
		})
	}
}
