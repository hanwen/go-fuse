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
	"path/filepath"
	"sync"
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
	if os.Geteuid() != 0 {
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
	dir := t.TempDir()

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
	l := len(bridge.kernelNodeIds)
	bridge.mu.Unlock()
	if l != 1 {
		t.Fatalf("got %d live nodes, want 1", l)
	}
}

type forgetTestRootNode struct {
	Inode

	mu            sync.Mutex
	onForgetCount int

	fileNode *forgetTestSubNode
	dirNode  *forgetTestSubNode
}

type forgetTestSubNode struct {
	Inode
	root *forgetTestRootNode
}

func (n *forgetTestSubNode) OnForget() {
	n.root.mu.Lock()
	defer n.root.mu.Unlock()
	n.root.onForgetCount++
}

func (n *forgetTestSubNode) OnAdd(ctx context.Context) {
	if !n.IsDir() {
		return
	}

	ftn := &forgetTestSubNode{root: n.root}
	ch := n.NewPersistentInode(ctx,
		ftn,
		StableAttr{Mode: fuse.S_IFREG})

	n.root.mu.Lock()
	defer n.root.mu.Unlock()
	n.root.dirNode = n
	n.root.fileNode = ftn
	n.AddChild("child", ch, true)
}

func (n *forgetTestRootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if name != "subdir" {
		return nil, syscall.ENOENT
	}
	if ch := n.GetChild(name); ch != nil {
		return ch, 0
	}

	ftn := &forgetTestSubNode{root: n}
	ch := n.NewInode(ctx, ftn,
		StableAttr{Mode: fuse.S_IFDIR})
	return ch, 0
}

func TestOnForget(t *testing.T) {
	root := &forgetTestRootNode{}
	mnt, _ := testMount(t, root, nil)

	sub := mnt + "/subdir"
	_, err := os.Stat(sub)
	if err != nil {
		t.Fatal(err)
	}
	root.mu.Lock()
	if root.dirNode == nil {
		t.Fatal("Lookup not triggered")
	}
	if root.fileNode == nil {
		t.Fatal("OnAdd not triggered")
	}
	root.mu.Unlock()
	if root.dirNode.GetChild("child") == nil {
		t.Fatal("child not there")
	}
	if err := syscall.Rmdir(sub); err != nil {
		t.Fatal(err)
	}
	if root.GetChild("subdir") != nil {
		t.Fatal("rmdir left node")
	}

	// The kernel issues FORGET immediately, but it takes a bit to
	// process the request.
	time.Sleep(time.Millisecond)
	root.mu.Lock()
	if root.onForgetCount != 0 {
		t.Errorf("got count %d, want 0", root.onForgetCount)
	}
	root.mu.Unlock()

	root.fileNode.ForgetPersistent()
	root.mu.Lock()
	if root.onForgetCount != 2 {
		t.Errorf("got count %d, want 2", root.onForgetCount)
	}
	root.mu.Unlock()
}
