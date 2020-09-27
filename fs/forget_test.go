// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type allChildrenNode struct {
	Inode

	depth int
}

var _ = (NodeLookuper)((*allChildrenNode)(nil))
var _ = (NodeReaddirer)((*allChildrenNode)(nil))

func (n *allChildrenNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if n.depth == 0 {
		return nil, syscall.ENOENT
	}
	stable := StableAttr{
		Mode: syscall.S_IFDIR,
	}

	if n.depth == 1 {
		stable.Mode = syscall.S_IFREG
	}
	childFN := &allChildrenNode{
		depth: n.depth - 1,
	}
	child := n.NewInode(ctx, childFN, stable)
	return child, 0
}

func (n *allChildrenNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	var list []fuse.DirEntry
	var m uint32 = syscall.S_IFDIR
	if n.depth == 1 {
		m = syscall.S_IFREG
	}
	for i := 0; i < 100; i++ {
		list = append(list, fuse.DirEntry{
			Name: fmt.Sprintf("%d", i),
			Mode: m,
		})
	}
	return NewListDirStream(list), 0
}

func TestForget(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if u.Uid != "0" {
		t.Skip("must run test as root")
	}
	root := &allChildrenNode{
		depth: 2,
	}
	sec := time.Second
	options := &Options{
		FirstAutomaticIno: 1,
		EntryTimeout:      &sec,
	}
	options.Debug = testutil.VerboseTest()
	dir, err := ioutil.TempDir("", "TestForget")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	rawFS := NewNodeFS(root, options)
	server, err := fuse.NewServer(rawFS, dir, &options.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}

	nop := func(path string, info os.FileInfo, err error) error {
		return nil
	}

	if err := filepath.Walk(dir, nop); err != nil {
		t.Fatal(err)
	}

	log.Println("dropping cache")
	if err := ioutil.WriteFile("/proc/sys/vm/drop_caches", []byte("2"), 0644); err != nil {

	}
	time.Sleep(time.Second)

	bridge := rawFS.(*rawBridge)
	bridge.mu.Lock()
	l := len(bridge.nodeidMap)
	bridge.mu.Unlock()
	if l != 1 {
		t.Fatalf("got %d live nodes, want 1", l)
	}
}
