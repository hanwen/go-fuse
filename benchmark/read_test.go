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
	doBenchmark(mnt, b, "direct", opRead)
}

const blockSize = 4 * 1024 * 1024 // 4M
const libfuseNumThreads = 32      // Number of libfuse worker threads
type ioOpType int

const (
	opRead ioOpType = iota
	opWrite
)

func doBenchmark(mnt string, b *testing.B, ddflag string, op ioOpType) {
	var inFile, outFile, bs, count, ioFlag string
	var cmds []*exec.Cmd
	parallelNum := runtime.GOMAXPROCS(0)

	switch op {
	case opRead:
		inFile = fmt.Sprintf("if=%s/foo.txt", mnt)
		outFile = "of=/dev/null"
		ioFlag = "iflag=" + ddflag
	case opWrite:
		inFile = "if=/dev/zero"
		ioFlag = "oflag=" + ddflag
	default:
		b.Errorf("Unsupported IO operate type: %v", op)
	}

	bs = fmt.Sprintf("bs=%d", blockSize)
	count = fmt.Sprintf("count=%d", b.N)

	for i := 0; i < parallelNum; i++ {
		if op == opWrite {
			outFile = fmt.Sprintf("of=%s/foo_%d.txt", mnt, i)
		}
		cmd := exec.Command("dd",
			inFile,
			outFile,
			bs,
			count)
		if ddflag != "" {
			cmd.Args = append(cmd.Args, ioFlag)
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

	b.SetBytes(int64(parallelNum * blockSize))
	b.ReportAllocs()
	b.ResetTimer()

	result := make(chan error, parallelNum)
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
		b.Errorf("%d out of %d commands", failures, parallelNum)
	}
	b.StopTimer()
}

func setupLoopbackFs(b *testing.B, createReadFile bool) string {
	orig := b.TempDir()

	if createReadFile {
		fn := orig + "/foo.txt"
		data := bytes.Repeat([]byte{42}, blockSize*b.N)
		if err := os.WriteFile(fn, data, 0666); err != nil {
			b.Fatal(err)
		}
	}

	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		b.Fatal(err)
	}
	mnt := setupFS(root, b.N, b)
	return mnt
}

func BenchmarkGoFuseFDRead(b *testing.B) {
	mnt := setupLoopbackFs(b, true)
	doBenchmark(mnt, b, "direct", opRead)
}

var libfusePath = flag.String("passthrough_hp", "", "path to libfuse's passthrough_hp")

func setupLibfuseFs(b *testing.B, createReadFile bool) string {
	orig := b.TempDir()
	mnt := b.TempDir()
	if *libfusePath == "" {
		b.Skip("must set --passthrough_hp")
	}

	origFN := orig + "/foo.txt"
	fn := mnt + "/foo.txt"

	if createReadFile {
		data := bytes.Repeat([]byte{42}, blockSize*b.N)
		if err := os.WriteFile(origFN, data, 0666); err != nil {
			b.Fatal(err)
		}
	} else { // create an empty file
		if _, err := os.Create(origFN); err != nil {
			b.Fatal(err)
		}
	}

	cmd := exec.Command(*libfusePath, "--foreground",
		fmt.Sprintf("--num-threads=%d", libfuseNumThreads),
		"--direct-io")
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

	return mnt
}

func BenchmarkLibfuseHPRead(b *testing.B) {
	mnt := setupLibfuseFs(b, true)
	doBenchmark(mnt, b, "", opRead)
}
