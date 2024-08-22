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
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func BenchmarkGoFuseMemoryRead(b *testing.B) {
	root := &readFS{}
	mnt := setupFS(root, b.N, b)
	benchmarkRead(mnt, b, 32, "direct")
}

const blockSize = 64 * 1024

func benchmarkRead(mnt string, b *testing.B, readers int, ddflag string) {
	var cmds []*exec.Cmd
	for i := 0; i < readers; i++ {
		cmd := exec.Command("dd",
			fmt.Sprintf("if=%s/foo.txt", mnt),
			"of=/dev/null",
			fmt.Sprintf("bs=%d", blockSize),
			fmt.Sprintf("count=%d", b.N))
		if ddflag != "" {
			cmd.Args = append(cmd.Args, "iflag="+ddflag)
		}
		if testutil.VerboseTest() {
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout
		} else {
			buf := &bytes.Buffer{}
			cmd.Stderr = buf
			cmd.Stdout = buf
		}
		cmds = append(cmds, cmd)
	}

	b.SetBytes(int64(readers * blockSize))
	b.ReportAllocs()
	b.ResetTimer()

	result := make(chan error, readers)
	for _, cmd := range cmds {
		go func(cmd *exec.Cmd) {
			err := cmd.Run()
			if buf, ok := cmd.Stdout.(*bytes.Buffer); ok && err != nil {
				err = fmt.Errorf("%v: output=%s", err, buf.String())
			}
			result <- err
		}(cmd)
	}
	failures := 0
	for range cmds {
		if err := <-result; err != nil {
			b.Errorf("dd failed: %v", err)
			failures++
		}
	}
	if failures > 0 {
		b.Errorf("%d out of %d commands", failures, readers)
	}
	b.StopTimer()
}

func BenchmarkGoFuseFDRead(b *testing.B) {
	orig := b.TempDir()
	fn := orig + "/foo.txt"

	data := bytes.Repeat([]byte{42}, blockSize*b.N)
	if err := os.WriteFile(fn, data, 0666); err != nil {
		b.Fatal(err)
	}
	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		b.Fatal(err)
	}
	mnt := setupFS(root, b.N, b)
	benchmarkRead(mnt, b, 1, "")
}

var libfusePath = flag.String("passthrough_hp", "", "path to libfuse's passthrough_hp")

func BenchmarkLibfuseHP(b *testing.B) {
	orig := b.TempDir()
	mnt := b.TempDir()
	if *libfusePath == "" {
		b.Skip("must set --passthrough_hp")
	}

	origFN := orig + "/foo.txt"
	data := bytes.Repeat([]byte{42}, blockSize*b.N)
	if err := os.WriteFile(origFN, data, 0666); err != nil {
		b.Fatal(err)
	}
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

	benchmarkRead(mnt, b, 1, "")
}
