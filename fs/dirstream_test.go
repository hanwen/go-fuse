// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type SimpleFS struct {
	Inode
}

func (fs *SimpleFS) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	const numOfEntries = 16

	entries := make([]fuse.DirEntry, 0, numOfEntries)
	for i := 0; i < numOfEntries; i++ {
		entries = append(entries, fuse.DirEntry{
			Name: fmt.Sprintf("name%04d", i),
			Mode: fuse.S_IFREG,
			Ino:  uint64(i + 100),
		})
	}
	return NewListDirStream(entries), 0
}

func TestDirSeek(t *testing.T) {
	mountDir, server := testMount(t, &SimpleFS{}, nil)
	defer os.RemoveAll(mountDir)
	defer server.Unmount()

	testDirSeek(t, mountDir)
}
