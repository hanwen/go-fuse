// Copyright 2021 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sync/errgroup"
)

func BenchmarkGoFuseMemoryRead(b *testing.B) {
	root := &readFS{}
	benchmarkGoFuseRead(root, b)
}

const blockSize = 64 * 1024

func benchmarkGoFuseRead(root fs.InodeEmbedder, b *testing.B) {
	wd, clean := setupFs(root, b.N)
	defer clean()

	jobs := 32
	cmds := make([]*exec.Cmd, jobs)
	for i := 0; i < jobs; i++ {
		cmds[i] = exec.Command("dd",
			fmt.Sprintf("if=%s/foo.txt", wd),
			"iflag=direct",
			"of=/dev/null",
			fmt.Sprintf("bs=%d", blockSize),
			fmt.Sprintf("count=%d", b.N))
		if testutil.VerboseTest() {
			cmds[i].Stdout = os.Stdout
			cmds[i].Stderr = os.Stderr
		}
	}

	b.SetBytes(int64(jobs * blockSize))
	b.ReportAllocs()
	b.ResetTimer()

	var eg errgroup.Group
	for i := 0; i < jobs; i++ {
		i := i
		eg.Go(func() error {
			return cmds[i].Run()
		})
	}

	if err := eg.Wait(); err != nil {
		b.Fatalf("dd failed: %v", err)
	}

	b.StopTimer()
}

func BenchmarkGoFuseFDRead(b *testing.B) {
	orig := b.TempDir()
	fn := orig + "/foo.txt"
	f, err := os.Create(fn)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	if err := f.Chmod(0777); err != nil {
		b.Fatal(err)
	}
	data := bytes.Repeat([]byte{42}, blockSize)
	for i := 0; i < b.N; i++ {
		_, err := f.Write(data)
		if err != nil {
			b.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		b.Fatal(err)
	}

	benchmarkGoFuseRead(root, b)
}
