// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"hash/crc64"
	"io/ioutil"
	"log"
	"os"
	"os/user"
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

	rawFS := NewNodeFS(root, options)
	server, err := fuse.NewServer(rawFS, dir, &options.MountOptions)
	if err != nil {
		t.Fatal(err)
	}

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
	l := len(bridge.nodes)
	bridge.mu.Unlock()
	if l != 1 {
		t.Fatalf("got %d live nodes, want 1", l)
	}
}

type notifyEntryTestRoot struct {
	Inode

	child *Inode

	once sync.Once
	quit chan struct{}
}

func (n *notifyEntryTestRoot) invalidate() {
	go func() {
	loop:
		for {
			select {
			case <-n.quit:
				break loop
			default:
			}

			n.NotifyEntry("TEST")
		}
	}()
}

type notifyEntryTestChild struct {
	Inode
}

var _ = (NodeOnAdder)((*notifyEntryTestRoot)(nil))

func (n *notifyEntryTestRoot) OnAdd(ctx context.Context) {
	n.child = n.NewInode(ctx, &notifyEntryTestChild{}, StableAttr{
		Ino:  42,
		Mode: syscall.S_IFDIR,
	})
}

var _ = (NodeLookuper)((*notifyEntryTestRoot)(nil))

func (n *notifyEntryTestRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.once.Do(n.invalidate)
	if name == "TEST" {
		return n.child, 0
	}

	return nil, syscall.ENOENT
}

var _ = (NodeLookuper)((*notifyEntryTestRoot)(nil))
var _ = (NodeReaddirer)((*notifyEntryTestChild)(nil))

func (n *notifyEntryTestChild) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	var list []fuse.DirEntry
	return NewListDirStream(list), 0
}

var _ = (NodeLookuper)((*notifyEntryTestChild)(nil))

var isoTable *crc64.Table = crc64.MakeTable(crc64.ISO)

func (n *notifyEntryTestChild) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	stable := StableAttr{
		Ino:  crc64.Checksum([]byte(name), isoTable),
		Mode: syscall.S_IFREG,
	}

	child := n.NewInode(ctx, &Inode{}, stable)
	return child, 0
}

// Exercises FORGET racing with LOOKUP. This tests the fix in 68f70527
// ("fs: addNewChild(): handle concurrent FORGETs")
func TestForgetLookup(t *testing.T) {
	root := &notifyEntryTestRoot{
		quit: make(chan struct{}),
	}
	sec := time.Second
	options := &Options{
		FirstAutomaticIno: 1,
		EntryTimeout:      &sec,
	}
	dir, err := ioutil.TempDir("", "TestForgetLookup")
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

	var wg sync.WaitGroup
	result := make(chan error, 100)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < 10; j++ {
				_, err := ioutil.ReadDir(dir + "/TEST")
				result <- err
			}

			wg.Done()
		}()
	}
	wg.Wait()

	close(root.quit)
}
