// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"flag"
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

// transportStats enables the per-mount transport counters printed when
// the server tears down. Off by default so benchmark output stays
// machine-parseable; set -transport-stats to inspect dispatch behavior.
var transportStats = flag.Bool("transport-stats", false,
	"log per-transport dispatch stats (CQEs, reads, wait/handle nanos) at mount teardown")

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
	return setupTreeOpts(tb, leaves, parallel, ioUring, time.Second, counter)
}

// setupTreeOpts is setupTree with an explicit cache timeout, for
// benchmarks that need to force every stat to round-trip to the server.
func setupTreeOpts(tb testing.TB, leaves, parallel int, ioUring bool, cacheTTL time.Duration, counter *atomic.Int64) string {
	root := &flatRoot{
		leaves:   leaves,
		parallel: parallel,
		counter:  counter,
	}
	opts := &fs.Options{}
	opts.Debug = testutil.VerboseTest()
	opts.EnableIoUring = ioUring
	opts.DebugTransportStats = *transportStats
	opts.DisableReadDirPlus = true
	opts.AttrTimeout = &cacheTTL
	opts.EntryTimeout = &cacheTTL
	opts.NegativeTimeout = &cacheTTL

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
		for _, p := range []int{16, 32} { // 1, 2, 4, 8} {
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

// BenchmarkStatDepth issues b.N Lstat calls across `depth` goroutines
// running in lockstep — each goroutine submits a stat, waits for the
// reply, submits the next. The kernel therefore sees ~depth requests
// in flight at all times, mimicking `fio --iodepth=N` for metadata.
//
// Unlike BenchmarkFindSize (where each `find` process serializes its
// own stats and yields to the kernel between calls), the in-process
// goroutine model keeps the request pipe saturated. That's the regime
// where uring's per-syscall amortization can pay off: CQE/enter should
// climb above 1 as depth grows.
//
// Run with -transport-stats to see the per-queue counters.
func BenchmarkStatDepth(b *testing.B) {
	// Pool of files large enough that consecutive stats from one
	// goroutine touch distinct inodes (no kernel attr-cache hit
	// short-circuit even with positive AttrTimeout).
	const fileCount = 8192
	for _, iou := range []bool{false, true} {
		for _, depth := range []int{1, 4, 16, 64, 256} {
			nm := ""
			if iou {
				nm += "uring,"
			}
			nm += fmt.Sprintf("depth=%d", depth)
			b.Run(nm, func(b *testing.B) {
				var counter atomic.Int64
				// Zero cache TTL: every stat round-trips. With
				// AttrTimeout>0 the kernel attr cache short-
				// circuits repeat stats of the same file, which
				// masks transport throughput differences.
				mnt := setupTreeOpts(b, fileCount, 1, iou, 0, &counter)
				sub := filepath.Join(mnt, "p0")

				per := b.N / depth
				if per < 1 {
					per = 1
				}
				b.ResetTimer()
				var wg sync.WaitGroup
				for w := 0; w < depth; w++ {
					wg.Add(1)
					go func(seed int) {
						defer wg.Done()
						var st syscall.Stat_t
						for j := 0; j < per; j++ {
							// 1009 is coprime with fileCount=8192/2,
							// so seeds spread evenly across the dir.
							idx := (seed*1009 + j) % fileCount
							_ = syscall.Stat(
								filepath.Join(sub, fmt.Sprintf("f%d", idx)),
								&st)
						}
					}(w)
				}
				wg.Wait()
				b.StopTimer()
				b.Logf("b.N=%d depth=%d per=%d stat-replies=%d", b.N, depth, per, counter.Load())
			})
		}
	}
}
