// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func setupFS(node fs.InodeEmbedder, N int, tb testing.TB) string {
	opts := &fs.Options{}
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

func readdir(d string) error {
	f, err := os.Open(d)
	if err != nil {
		return err
	}
	if _, err := f.Readdirnames(-1); err != nil {
		return err
	}
	return f.Close()
}

func BenchmarkGoFSReaddir(b *testing.B) {
	b.StopTimer()
	fs := &StatFS{}

	wd, _ := os.Getwd()
	dirSet := map[string]struct{}{}

	for _, fn := range ReadLines(wd + "/testpaths.txt") {
		fs.AddFile(fn, fuse.Attr{Mode: syscall.S_IFREG})
		dirSet[filepath.Dir(fn)] = struct{}{}
	}

	mnt := setupFS(fs, b.N, b)

	var dirs []string
	for dir := range dirSet {
		dirs = append(dirs, filepath.Join(mnt, dir))
	}
	b.StartTimer()
	todo := b.N
	for todo > 0 {
		if len(dirs) > todo {
			dirs = dirs[:todo]
		}
		for _, d := range dirs {
			if err := readdir(d); err != nil {
				b.Fatal(err)
			}
		}
		todo -= len(dirs)
	}
	b.StopTimer()
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
