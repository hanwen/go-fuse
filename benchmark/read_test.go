// Copyright 2021 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func BenchmarkGoFuseMemoryRead(b *testing.B) {
	root := &readFS{}
	mnt := setupFS(root, b.N, b)
	benchmarkIO(mnt, b, false, true)
}

func BenchmarkGoFuseMemoryWrite(b *testing.B) {
	root := &readFS{}
	mnt := setupFS(root, b.N, b)
	benchmarkIO(mnt, b, true, true)
}

const blockSize = 128 * 1024 // Fuse default request size 128k

func benchmarkRead(mnt string, b *testing.B, direct bool) {
	benchmarkIO(mnt, b, false, direct)
}

// benchmarkIO has GOMAXPROCS threads each read b.N blocks of
// foo.txt, or write b.N blocks to their own file, in mnt.
func benchmarkIO(mnt string, b *testing.B, write, direct bool) {
	threads := runtime.GOMAXPROCS(0)
	mode, name := "-mode=read", "foo.txt"
	if write {
		mode, name = "-mode=write", "foo%d.txt"
	}
	b.SetBytes(int64(threads * blockSize))
	b.ReportAllocs()
	runBulkstat(b, mode, threads, b.N, mnt,
		fmt.Sprintf("-bs=%d", blockSize),
		fmt.Sprintf("-direct=%v", direct),
		name)
}

func writeBlocks(b *testing.B, name string, n int) {
	f, err := os.Create(name)
	if err != nil {
		b.Fatal(err)
	}
	block := bytes.Repeat([]byte{42}, blockSize)
	for i := 0; i < n; i++ {
		if _, err := f.Write(block); err != nil {
			b.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkGoFuseFDRead(b *testing.B) {
	orig := b.TempDir()
	writeBlocks(b, orig+"/foo.txt", b.N)
	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		b.Fatal(err)
	}
	mnt := setupFS(root, b.N, b)
	benchmarkRead(mnt, b, false)
}

var libfusePath = flag.String("passthrough_hp", "", "path to libfuse's passthrough_hp")

func BenchmarkLibfuseHP(b *testing.B) {
	orig := b.TempDir()
	mnt := b.TempDir()
	if *libfusePath == "" {
		b.Skip("must set --passthrough_hp")
	}

	writeBlocks(b, orig+"/foo.txt", b.N)
	fn := mnt + "/foo.txt"
	cmd := exec.Command(*libfusePath, "--foreground")
	if testutil.VerboseTest() {
		cmd.Args = append(cmd.Args, "--debug", "--debug-fuse")
	}
	cmd.Args = append(cmd.Args, orig, mnt)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { exec.Command("fusermount", "-u", mnt).Run() })

	dt := time.Millisecond
	for {
		if _, err := os.Stat(fn); err == nil {
			break
		}
		time.Sleep(dt)
		dt *= 2
		if dt > time.Second {
			b.Fatal("file did not appear")
		}
	}

	benchmarkRead(mnt, b, false)
}
