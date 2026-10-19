// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func setupFS(node fs.InodeEmbedder, N int, tb testing.TB) string {
	return setupFSWithOptions(node, &fs.Options{}, tb)
}

func setupFSWithOptions(node fs.InodeEmbedder, opts *fs.Options, tb testing.TB) string {
	opts.Debug = testutil.VerboseTest()
	mountPoint := tb.TempDir()
	server, err := fs.Mount(mountPoint, node, opts)
	if err != nil {
		tb.Fatalf("cannot mount %v", err)
	}
	tb.Cleanup(func() {
		err := server.Unmount()
		if err != nil {
			log.Println("error during unmount", err)
		}
	})
	return mountPoint
}

func TestNewStatFs(t *testing.T) {
	fs := &StatFS{}
	for _, n := range []string{
		"file.txt", "sub/dir/foo.txt",
		"sub/dir/bar.txt", "sub/marine.txt"} {
		fs.AddFile(n, fuse.Attr{Mode: syscall.S_IFREG})
	}

	wd := setupFS(fs, 1, t)

	names, err := os.ReadDir(wd)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(names) != 2 {
		t.Error("readdir /", names)
	}

	fi, err := os.Lstat(wd + "/sub")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !fi.IsDir() {
		t.Error("mode", fi)
	}
	names, err = os.ReadDir(wd + "/sub")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(names) != 2 {
		t.Error("readdir /sub", names)
	}
	names, err = os.ReadDir(wd + "/sub/dir")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(names) != 2 {
		t.Error("readdir /sub/dir", names)
	}

	fi, err = os.Lstat(wd + "/sub/marine.txt")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if fi.Mode()&os.ModeType != 0 {
		t.Error("mode", fi)
	}
}

func BenchmarkGoFSStat(b *testing.B) {
	b.StopTimer()
	fs := &StatFS{}

	wd, _ := os.Getwd()
	fileList := wd + "/testpaths.txt"
	files := ReadLines(fileList)
	for _, fn := range files {
		fs.AddFile(fn, fuse.Attr{Mode: syscall.S_IFREG})
	}

	mnt := setupFS(fs, b.N, b)

	for i, l := range files {
		files[i] = filepath.Join(mnt, l)
	}

	threads := runtime.GOMAXPROCS(0)
	if err := TestingBOnePass(b, threads, fileList, mnt); err != nil {
		b.Fatalf("TestingBOnePass %v8", err)
	}
}

func readdir(d string) (int, error) {
	f, err := os.Open(d)
	if err != nil {
		return 0, err
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		f.Close()
		return 0, err
	}
	return len(names), f.Close()
}

func BenchmarkGoFSParallelReaddir(b *testing.B) {
	root := &StatFS{}
	for i := 0; i < 16; i++ {
		root.AddFile(fmt.Sprintf("file%02d", i), fuse.Attr{Mode: syscall.S_IFREG})
	}
	mnt := setupFS(root, b.N, b)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Avoid client-side allocations for directory entry names.
		buf := make([]byte, 32768)
		run := func() (err error) {
			fd, err := syscall.Open(mnt, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)
			for {
				n, err := syscall.ReadDirent(fd, buf)
				if err != nil {
					return err
				}
				if n == 0 {
					return nil
				}
			}
		}
		for pb.Next() {
			if err := run(); err != nil {
				b.Error(err)
				return
			}
		}
	})
	b.StopTimer()
}

// BenchmarkReadDirEntCompleteDir lists a directory opened with
// FOPEN_COMPLETE_DIR, so every pass is READDIRPLUS and builds a listing.
func BenchmarkReadDirEntCompleteDir(b *testing.B) {
	const N = 500
	root := &StatFS{}
	for i := 0; i < N; i++ {
		root.AddFile(fmt.Sprintf("file%03d", i), fuse.Attr{Mode: syscall.S_IFREG})
	}
	mnt, _ := setupCompleteDirFS(root, b)

	b.ResetTimer()
	total := 0
	for total < b.N {
		n, err := readdir(mnt)
		if err != nil {
			b.Fatal(err)
		}
		if n != N {
			b.Fatalf("got %d entries, want %d", n, N)
		}
		total += n
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(total), "ns/op")
}

func benchmarkNegStat(b *testing.B, opts *fs.Options) {
	mnt := setupFSWithOptions(&StatFS{}, opts, b)
	fn := mnt + "/nonexistent"

	var st syscall.Stat_t
	if err := syscall.Stat(fn, &st); err != syscall.ENOENT {
		b.Fatalf("got %v, want ENOENT", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := syscall.Stat(fn, &st); err != syscall.ENOENT {
			b.Fatalf("got %v, want ENOENT", err)
		}
	}
	b.StopTimer()
}

func BenchmarkNegStatUncached(b *testing.B) {
	benchmarkNegStat(b, &fs.Options{})
}

func BenchmarkNegStatCached(b *testing.B) {
	dt := time.Hour
	benchmarkNegStat(b, &fs.Options{
		NegativeTimeout: &dt,
	})
}

// completeDirFS marks all directories with FOPEN_COMPLETE_DIR and counts
// LOOKUPs.
type completeDirFS struct {
	fuse.RawFileSystem
	server   *fuse.Server
	lookups  atomic.Int64
	released chan struct{}
}

func (c *completeDirFS) ReleaseDir(ctx context.Context, input *fuse.ReleaseIn) {
	c.RawFileSystem.ReleaseDir(ctx, input)
	select {
	case c.released <- struct{}{}:
	default:
	}
}

func (c *completeDirFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	c.lookups.Add(1)
	return c.RawFileSystem.Lookup(ctx, header, name, out)
}

func (c *completeDirFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	st := c.RawFileSystem.OpenDir(ctx, input, out)
	if st.Ok() {
		out.OpenFlags |= fuse.FOPEN_COMPLETE_DIR
	}
	return st
}

func setupCompleteDirFS(root fs.InodeEmbedder, b *testing.B) (string, *completeDirFS) {
	dt := time.Hour
	opts := &fs.Options{
		EntryTimeout: &dt,
		AttrTimeout:  &dt,
	}
	opts.Debug = testutil.VerboseTest()
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR
	cfs := &completeDirFS{
		RawFileSystem: fs.NewNodeFS(root, opts),
		released:      make(chan struct{}, 1),
	}

	mnt := b.TempDir()
	server, err := fuse.NewServer(cfs, mnt, &opts.MountOptions)
	if err != nil {
		b.Fatal(err)
	}
	cfs.server = server
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { server.Unmount() })
	if server.KernelSettings().Flags64()&fuse.CAP_HAS_COMPLETE_DIR == 0 {
		b.Log("kernel does not support CAP_HAS_COMPLETE_DIR")
	}
	return mnt, cfs
}

// BenchmarkNegStatUncachedDir stats absent names in a directory that was
// listed with FOPEN_COMPLETE_DIR. Kernels that support CAP_HAS_COMPLETE_DIR
// answer these without sending LOOKUP.
func BenchmarkNegStatUncachedDir(b *testing.B) {
	const N = 500
	root := &StatFS{}
	for i := 0; i < N; i++ {
		root.AddFile(fmt.Sprintf("file%03d", i), fuse.Attr{Mode: syscall.S_IFREG})
	}
	mnt, cfs := setupCompleteDirFS(root, b)

	if _, err := os.ReadDir(mnt); err != nil {
		b.Fatal(err)
	}

	prefix := mnt + "/nonexistent"
	var st syscall.Stat_t
	before := cfs.lookups.Load()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn := prefix + strconv.Itoa(i)
		if err := syscall.Stat(fn, &st); err != syscall.ENOENT {
			b.Fatalf("got %v, want ENOENT", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(cfs.lookups.Load()-before)/float64(b.N), "lookups/op")
}

// BenchmarkReaddirplusHint drops the listing of a directory, and has the
// kernel read it again with FUSE_NOTIFY_READDIRPLUS. The ns/op metric is
// per entry.
func BenchmarkReaddirplusHint(b *testing.B) {
	const N = 500
	root := &StatFS{}
	for i := 0; i < N; i++ {
		root.AddFile(fmt.Sprintf("file%03d", i), fuse.Attr{Mode: syscall.S_IFREG})
	}
	_, cfs := setupCompleteDirFS(root, b)
	if !cfs.server.KernelSettings().SupportsNotify(fuse.NOTIFY_READDIRPLUS) {
		b.Skip("kernel does not support NOTIFY_READDIRPLUS")
	}

	b.ResetTimer()
	total := 0
	for total < b.N {
		if st := cfs.server.InodeNotify(fuse.FUSE_ROOT_ID, 0, 0); !st.Ok() {
			b.Fatalf("InodeNotify: %v", st)
		}
		// EALREADY until the previous read has finished in the
		// kernel, which is after it sent RELEASEDIR.
		st := cfs.server.ReaddirplusNotify(fuse.FUSE_ROOT_ID)
		for st == fuse.Status(syscall.EALREADY) {
			st = cfs.server.ReaddirplusNotify(fuse.FUSE_ROOT_ID)
		}
		if !st.Ok() {
			b.Fatalf("ReaddirplusNotify: %v", st)
		}
		<-cfs.released
		total += N
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(total), "ns/op")
}

func TestingBOnePass(b *testing.B, threads int, filelist, mountPoint string) error {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// We shell out to an external program, so the time spent by
	// the part stat'ing doesn't interfere with the time spent by
	// the FUSE server.
	cmd := exec.Command("./bulkstat.bin",
		fmt.Sprintf("-cpu=%d", threads),
		fmt.Sprintf("-prefix=%s", mountPoint),
		fmt.Sprintf("-N=%d", b.N),
		fmt.Sprintf("-quiet=%v", !testutil.VerboseTest()),
		filelist)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	b.StartTimer()
	err := cmd.Run()
	b.StopTimer()
	runtime.ReadMemStats(&after)
	if err != nil {
		return err
	}

	if testutil.VerboseTest() {
		fmt.Printf("GC count %d, total GC time: %d ns/file\n",
			after.NumGC-before.NumGC, (after.PauseTotalNs-before.PauseTotalNs)/uint64(b.N))
	}
	return nil
}

func BenchmarkLibfuseHighlevelThreadedStat(b *testing.B) {
	b.StopTimer()

	wd, _ := os.Getwd()
	if _, err := os.Stat(wd + "/cstatfs"); err != nil {
		b.Skip(err)
	}
	fileList := wd + "/testpaths.txt"
	lines := ReadLines(fileList)
	unique := map[string]int{}
	for _, l := range lines {
		unique[l] = 1
		dir, _ := filepath.Split(l)
		for dir != "/" && dir != "" {
			unique[dir] = 1
			dir = filepath.Clean(dir)
			dir, _ = filepath.Split(dir)
		}
	}

	out := []string{}
	for k := range unique {
		out = append(out, k)
	}

	f, err := os.CreateTemp("", "")
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	sort.Strings(out)
	for _, k := range out {
		f.Write([]byte(fmt.Sprintf("/%s\n", k)))
	}
	f.Close()

	mountPoint, err := os.MkdirTemp("", "")
	if err != nil {
		b.Fatalf("MkdirTemp: %v", err)
	}

	cmd := exec.Command(wd+"/cstatfs",
		"-o",
		"entry_timeout=0.0,attr_timeout=0.0,ac_attr_timeout=0.0,negative_timeout=0.0",
		mountPoint)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("STATFS_INPUT=%s", f.Name()))
	cmd.Start()

	bin, err := exec.LookPath("fusermount")
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	stop := exec.Command(bin, "-u", mountPoint)
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	defer stop.Run()

	time.Sleep(100 * time.Millisecond)
	os.Lstat(mountPoint)
	threads := runtime.GOMAXPROCS(0)
	if err := TestingBOnePass(b, threads, fileList, mountPoint); err != nil {
		b.Fatalf("TestingBOnePass %v", err)
	}
}
