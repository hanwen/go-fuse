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

type direntTest struct {
	Inode

	nameModeMap map[string]uint32
}

func (fs *direntTest) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(fs.nameModeMap))
	for k, v := range fs.nameModeMap {
		entries = append(entries, fuse.DirEntry{
			Name: k,
			Mode: v,
			Ino:  uint64(v + 100),
		})
	}
	return NewListDirStream(entries), 0
}

func TestDirentType(t *testing.T) {
	root := &direntTest{
		nameModeMap: map[string]uint32{
			"a":       syscall.S_IFDIR,
			"bb":      syscall.S_IFREG,
			"lnk":     syscall.S_IFLNK,
			"dddd":    syscall.S_IFCHR,
			"eeeee":   syscall.S_IFSOCK,
			"ffffff":  syscall.S_IFIFO,
			"ggggggg": syscall.S_IFBLK,
		},
	}
	opts := Options{}
	opts.DisableReadDirPlus = true
	mountDir, server := testMount(t, root, &opts)
	defer os.RemoveAll(mountDir)
	defer server.Unmount()

	ds, errno := NewLoopbackDirStream(mountDir)
	if errno != 0 {
		t.Fatalf("Next: %v", errno)
	}
	defer ds.Close()
	for {
		if !ds.HasNext() {
			break
		}
		de, errno := ds.Next()
		if errno != 0 {
			t.Fatalf("Next: %v", errno)
		}
		if de.Mode != root.nameModeMap[de.Name] {
			t.Errorf("%s: got mode %o want mode %o", de.Name, de.Mode, root.nameModeMap[de.Name])
		}
	}
}
