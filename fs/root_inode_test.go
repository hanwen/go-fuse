// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type fsTestCase struct {
	*testing.T

	dir     string
	origDir string
	mntDir  string

	loopback fs.InodeEmbedder
	server   *fuse.Server
}

// writeOrig writes a file into the backing directory of the loopback mount
func (tc *fsTestCase) writeOrig(path, content string, mode os.FileMode) {
	if err := os.WriteFile(filepath.Join(tc.origDir, path), []byte(content), mode); err != nil {
		tc.Fatal(err)
	}
}

func (tc *fsTestCase) Clean() {
	if err := tc.server.Unmount(); err != nil {
		tc.Fatal(err)
	}
	if err := os.RemoveAll(tc.dir); err != nil {
		tc.Fatal(err)
	}
}

func newFsTestCase(t *testing.T, rootIno uint64) *fsTestCase {
	tc := &fsTestCase{
		dir: testutil.TempDir(),
		T:   t,
	}
	tc.origDir = tc.dir + "/orig"
	tc.mntDir = tc.dir + "/mnt"
	if err := os.Mkdir(tc.origDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tc.mntDir, 0755); err != nil {
		t.Fatal(err)
	}

	var err error
	tc.loopback, err = fs.NewLoopbackRoot(tc.origDir)
	if err != nil {
		t.Fatalf("NewLoopback: %v", err)
	}

	sec := time.Second
	opts := &fs.Options{
		AttrTimeout:  &sec,
		EntryTimeout: &sec,
		RootStableAttr: &fs.StableAttr{
			Ino: rootIno,
			Gen: 1,
		},
	}
	tc.server, err = fs.Mount(tc.mntDir, tc.loopback, opts)
	if err != nil {
		t.Fatal(err)
	}

	return tc
}

func TestRootInode(t *testing.T) {
	var rootIno uint64 = 42
	tc := newFsTestCase(t, rootIno)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)
	tc.writeOrig("dir/file", "x", 0644)

	root := tc.loopback.EmbeddedInode().Root()

	if root.StableAttr().Ino != rootIno {
		t.Fatalf("root inode set fail: expect %v got %v\n", rootIno, root.StableAttr().Ino)
	}

	fileinfo, err := os.Stat(tc.mntDir)
	if err != nil {
		t.Fatal(err)
	}
	sys := fileinfo.Sys()
	stat, ok := sys.(*syscall.Stat_t)
	if !ok {
		t.Fatalf("syscall stat fail\n")
	}
	if stat.Ino != rootIno {
		t.Fatalf("root inode set fail: expect %v got %v\n", rootIno, stat.Ino)
	}
}
