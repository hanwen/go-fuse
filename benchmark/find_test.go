// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// countingFile is a regular file whose Getattr bumps a shared counter.
// Used to verify the kernel is actually round-tripping each stat to us
// (vs. servicing it from the FUSE attr cache).
type countingFile struct {
	fs.Inode
	size    uint64
	counter *atomic.Int64
}

var _ = (fs.NodeGetattrer)((*countingFile)(nil))

func (f *countingFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	f.counter.Add(1)
	out.Mode = syscall.S_IFREG | 0644
	out.Size = f.size
	return 0
}

// flatRoot lays out r.leaves countingFiles as a flat list, split evenly
// across r.parallel top-level "pX" subdirs. Each subdir holds N/P files
// directly — no nested directories — so a `find -size +5` over pX
// produces exactly N/P Getattr calls.
type flatRoot struct {
	fs.Inode
	leaves   int
	parallel int
	counter  *atomic.Int64
}

var _ = (fs.NodeOnAdder)((*flatRoot)(nil))

func (r *flatRoot) OnAdd(ctx context.Context) {
	remaining := r.leaves
	for i := 0; i < r.parallel; i++ {
		share := remaining / (r.parallel - i)
		remaining -= share
		dir := r.EmbeddedInode().NewPersistentInode(ctx, &fs.Inode{},
			fs.StableAttr{Mode: syscall.S_IFDIR})
		r.EmbeddedInode().AddChild(fmt.Sprintf("p%d", i), dir, true)
		for j := 0; j < share; j++ {
			size := uint64(1)
			if j%2 == 0 {
				size = 16
			}
			child := dir.NewPersistentInode(ctx, &countingFile{
				size:    size,
				counter: r.counter,
			}, fs.StableAttr{Mode: syscall.S_IFREG})
			dir.AddChild(fmt.Sprintf("f%d", j), child, true)
		}
	}
}

func setupTree(tb testing.TB, leaves, parallel int, ioUring bool, counter *atomic.Int64) string {
	root := &flatRoot{
		leaves:   leaves,
		parallel: parallel,
		counter:  counter,
	}
	opts := &fs.Options{}
	opts.Debug = testutil.VerboseTest()
	opts.EnableIoUring = ioUring
	opts.DisableReadDirPlus = true
	sec := time.Second
	opts.AttrTimeout = &sec
	opts.EntryTimeout = &sec
	opts.NegativeTimeout = &sec

	mnt := tb.TempDir()
	srv, err := fs.Mount(mnt, root, opts)
	if err != nil {
		tb.Fatalf("mount: %v", err)
	}
	tb.Cleanup(func() { srv.Unmount() })
	if ioUring && srv.KernelSettings().Flags64()&fuse.CAP_OVER_IO_URING == 0 {
		tb.Skip("kernel does not advertise CAP_OVER_IO_URING")
	}
	return mnt
}

// BenchmarkFindSize times P parallel `find -size +5` passes over a flat
// list of b.N files (split N/P per subdir). One Getattr fires per file,
// so total Getattr calls ≈ b.N. The reported ns/op is wall time per
// stat, which lets the framework auto-tune to a stable measurement.
//
// The workload is all metadata (LOOKUP/GETATTR/READDIRPLUS), which is
// where the uring transport should win against the classic /dev/fuse
// round-trip.
func BenchmarkFindSize(b *testing.B) {
	for _, iou := range []bool{false, true} {
		for _, p := range []int{1, 2, 4, 8} {
			nm := ""
			if iou {
				nm += "uring,"
			}
			nm += fmt.Sprintf("P=%d", p)
			b.Run(nm, func(b *testing.B) {
				var counter atomic.Int64
				mnt := setupTree(b, b.N, p, iou, &counter)

				b.ResetTimer()
				var wg sync.WaitGroup
				errs := make([]error, p)
				for i := 0; i < p; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						sub := filepath.Join(mnt, fmt.Sprintf("p%d", i))
						errs[i] = exec.Command("find", sub, "-size", "+5").Run()
					}(i)
				}
				wg.Wait()
				b.StopTimer()
				for i, err := range errs {
					if err != nil {
						b.Fatalf("find p%d: %v", i, err)
					}
				}
				b.Logf("b.N=%d P=%d Getattr calls: %d", b.N, p, counter.Load())
			})
		}
	}
}
