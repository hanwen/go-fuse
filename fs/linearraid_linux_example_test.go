// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package fs_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/splice"
)

// linearRaidNode presents a single file assembled from fixed-size chunk files.
// Each Read splices from the appropriate chunk file into a splice.Pair and
// returns it via fuse.ReadResultPipe, exercising the pipe-backed ReadResult
// path including Done()/discard().
type linearRaidNode struct {
	fs.Inode
	chunks    []string // paths to chunk files on disk
	chunkSize int
	size      int64
}

var _ = (fs.NodeGetattrer)((*linearRaidNode)(nil))
var _ = (fs.NodeOpener)((*linearRaidNode)(nil))
var _ = (fs.NodeReader)((*linearRaidNode)(nil))

func (n *linearRaidNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *linearRaidNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0444
	out.Size = uint64(n.size)
	return 0
}

func (n *linearRaidNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off >= n.size {
		return fuse.ReadResultData(nil), 0
	}

	// Total bytes to serve, clamped to file size and dest buffer.
	end := off + int64(len(dest))
	if end > n.size {
		end = n.size
	}
	total := int(end - off)

	pair, err := splice.Get()
	if err != nil {
		return nil, syscall.EIO
	}
	cleanup := func() { splice.Done(pair) }
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err := pair.Grow(total); err != nil {
		return nil, syscall.EIO
	}

	// Splice each chunk segment covering [off, end) into the pipe in order.
	cur := off
	for cur < end {
		chunkIdx := int(cur) / n.chunkSize
		chunkOff := int64(int(cur) % n.chunkSize)
		sz := n.chunkSize - int(chunkOff)
		if cur+int64(sz) > end {
			sz = int(end - cur)
		}

		chunkFd, err := syscall.Open(n.chunks[chunkIdx], syscall.O_RDONLY, 0)
		if err != nil {
			return nil, syscall.EIO
		}
		n2, err := pair.LoadFromAt(uintptr(chunkFd), sz, chunkOff)
		syscall.Close(chunkFd)
		if err != nil || n2 == 0 {
			return nil, syscall.EIO
		}
		cur += int64(n2)
	}

	cleanup = nil
	return fuse.ReadResultPipe(pair, total), 0
}

// Example_linearRaid demonstrates assembling a virtual file from fixed-size
// chunk files on disk using zero-copy splice. Each Read call splices the
// relevant chunk segments into a pipe and returns it via fuse.ReadResultPipe.
func Example_linearRaid() {
	const (
		chunkSize = 64 * 1024 // 64 KiB
		numChunks = 4
	)
	totalSize := int64(chunkSize * numChunks)

	// Write chunk files with distinct, recognisable content.
	chunkDir, err := os.MkdirTemp("", "chunks")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(chunkDir)

	var chunks []string
	var want []byte
	for i := 0; i < numChunks; i++ {
		data := bytes.Repeat([]byte{byte(i + 1)}, chunkSize)
		want = append(want, data...)
		p := filepath.Join(chunkDir, fmt.Sprintf("chunk%d", i))
		if err := os.WriteFile(p, data, 0644); err != nil {
			log.Fatal(err)
		}
		chunks = append(chunks, p)
	}

	mntDir, err := os.MkdirTemp("", "mnt")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(mntDir)

	root := &fs.Inode{}
	opts := &fs.Options{
		FirstAutomaticIno: 1,
		OnAdd: func(ctx context.Context) {
			n := root.EmbeddedInode()
			ch := n.NewPersistentInode(ctx, &linearRaidNode{
				chunks:    chunks,
				chunkSize: chunkSize,
				size:      totalSize,
			}, fs.StableAttr{})
			n.AddChild("raid", ch, false)
		},
	}
	// opts.Debug = true
	server, err := fs.Mount(mntDir, root, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Unmount()

	got, err := os.ReadFile(mntDir + "/raid")
	if err != nil {
		log.Fatal(err)
	}
	if bytes.Equal(got, want) {
		fmt.Println("content matches")
	} else {
		fmt.Printf("content mismatch: got %d bytes, want %d\n", len(got), len(want))
	}
	// Output:
	// content matches
}
