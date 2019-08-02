// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestReadonlyCreate(t *testing.T) {
	root := &Inode{}

	mntDir, _, clean := testMount(t, root, nil)
	defer clean()

	_, err := syscall.Creat(mntDir+"/test", 0644)
	if want := syscall.EROFS; want != err {
		t.Fatalf("got err %v, want %v", err, want)
	}
}

func TestDefaultPermissions(t *testing.T) {
	root := &Inode{}

	mntDir, _, clean := testMount(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			dir := root.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFDIR})
			file := root.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFREG})

			root.AddChild("dir", dir, false)
			root.AddChild("file", file, false)
		},
	})
	defer clean()

	for k, v := range map[string]uint32{
		"dir":  fuse.S_IFDIR | 0755,
		"file": fuse.S_IFREG | 0644,
	} {
		var st syscall.Stat_t
		if err := syscall.Lstat(filepath.Join(mntDir, k), &st); err != nil {
			t.Error("Lstat", err)
		} else if st.Mode != v {
			t.Errorf("got %o want %o", st.Mode, v)
		}
	}
}
