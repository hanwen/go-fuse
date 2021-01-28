// Copyright 2021 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sync/errgroup"
)

func BenchmarkGoFuseRead(b *testing.B) {
	fs := &readFS{}
	wd, clean := setupFs(fs, b.N)
	defer clean()

	jobs := 32
	blockSize := 64 * 1024

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
