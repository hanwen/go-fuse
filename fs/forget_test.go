// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
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
	if !testutil.RealRoot() {
		t.Skip("must run test as root")
	}
	if runtime.GOOS != "linux" {
		t.Skip("test needs linux")
	}
	root := &allChildrenNode{
		depth: 2,
	}
	ttl := 10 * time.Millisecond
	options := &Options{
		FirstAutomaticIno: 1,
		EntryTimeout:      &ttl,
	}
	options.Debug = testutil.VerboseTest()
	options.PanicHandler = testutil.PanicHandler(t, fuse.EIO)
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

	t.Log("dropping cache")
	if err := os.WriteFile("/proc/sys/vm/drop_caches", []byte("2"), 0644); err != nil {

	}

	bridge := rawFS.(*rawBridge)
	liveNodes := func() int {
		bridge.ids.mu.Lock()
		defer bridge.ids.mu.Unlock()
		return len(bridge.ids.nodes)
	}

	last := liveNodes()
	for last != 1 {
		time.Sleep(ttl)
		l := liveNodes()
		if l >= last {
			t.Fatalf("live nodes stuck at %d, want 1", l)
		}
		last = l
	}
}

type forgetTestRootNode struct {
	Inode

	mu            sync.Mutex
	onForgetCount uint32

	fileNode *forgetTestSubNode
	dirNode  *forgetTestSubNode
}

func (n *forgetTestRootNode) OnForget() {
	atomic.AddUint32(&n.onForgetCount, 1)
}

type forgetTestSubNode struct {
	Inode
	root *forgetTestRootNode
}

func (n *forgetTestSubNode) OnForget() {
	n.root.OnForget()
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
	mnt, srv := testMount(t, root, nil)

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
	if got := atomic.LoadUint32(&root.onForgetCount); got != 0 {
		t.Errorf("got count %d, want 0", got)
	}

	root.fileNode.ForgetPersistent()
	if got := atomic.LoadUint32(&root.onForgetCount); got != 2 {
		t.Errorf("got count %d, want 2", got)
	}

	if err := srv.Unmount(); err != nil {
		t.Errorf("Unmount: %v", err)
	}

	// The FORGET processing is not synchronized, so goroutine
	// scheduling may cause it happen after the unmount fully completes.
	time.Sleep(time.Millisecond)
	if got := atomic.LoadUint32(&root.onForgetCount); got != 3 {
		t.Errorf("got count %d, want 3", got)
	}
}

// exclusiveForgetRoot simulates a backing store that reuses inode
// numbers: once "a" has been deleted behind the kernel's back, a newly
// created file gets a's inode number.
type exclusiveForgetRoot struct {
	Inode

	mu        sync.Mutex
	aExists   bool
	forgotten chan struct{}
}

type exclusiveForgetFile struct {
	Inode

	forgotten chan struct{}
}

func (n *exclusiveForgetFile) OnForget() {
	if n.forgotten != nil {
		close(n.forgotten)
	}
}

const exclusiveForgetIno = 42

func (n *exclusiveForgetRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if name != "a" || !n.aExists {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(ctx, &exclusiveForgetFile{forgotten: n.forgotten},
		StableAttr{Mode: fuse.S_IFREG, Ino: exclusiveForgetIno})
	return ch, 0
}

func (n *exclusiveForgetRoot) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*Inode, FileHandle, uint32, syscall.Errno) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.aExists {
		return nil, nil, 0, syscall.ENOSPC
	}
	ch := n.NewInode(ctx, &exclusiveForgetFile{},
		StableAttr{Mode: fuse.S_IFREG, Ino: exclusiveForgetIno})
	return ch, nil, 0, 0
}

// TestForgetAfterExclusiveRegistration checks that the nodeid of a node
// whose StableAttr was taken over by an exclusive (O_EXCL) registration
// is released when the kernel forgets it.
func TestForgetAfterExclusiveRegistration(t *testing.T) {
	root := &exclusiveForgetRoot{aExists: true, forgotten: make(chan struct{})}
	// Without ExternalNodeID, so the O_EXCL node gets a fresh
	// nodeid. Zero timeouts make the kernel revalidate "a".
	mnt, _ := testMount(t, root, &Options{})

	a := filepath.Join(mnt, "a")
	if _, err := os.Lstat(a); err != nil {
		t.Fatal(err)
	}
	n1 := root.GetChild("a")
	if n1 == nil {
		t.Fatal("a not in tree")
	}

	// "a" is deleted in the backing store, not through the mount.
	root.mu.Lock()
	root.aExists = false
	root.mu.Unlock()

	f, err := os.OpenFile(filepath.Join(mnt, "b"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if n2 := root.GetChild("b"); n2 == nil || n2 == n1 || n2.nodeId == n1.nodeId {
		t.Fatalf("got b=%v, want a different node than a=%v", n2, n1)
	}

	// The failed revalidation makes the kernel drop a's nodeid.
	if _, err := os.Lstat(a); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(a): got %v, want ENOENT", err)
	}
	select {
	case <-root.forgotten:
	case <-time.After(5 * time.Second):
		t.Fatal("kernel did not forget a")
	}

	bridge := root.EmbeddedInode().bridge
	bridge.ids.mu.Lock()
	_, ok := bridge.ids.nodes[n1.nodeId]
	bridge.ids.mu.Unlock()
	if ok {
		t.Errorf("nodeid %d still registered after FORGET", n1.nodeId)
	}
}

func TestNotifyEntryForgetsSubtree(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test needs linux")
	}
	root := &allChildrenNode{depth: 2}
	ttl := time.Hour
	mnt, _ := testMount(t, root, &Options{EntryTimeout: &ttl})

	for _, d := range []string{"0", "1"} {
		entries, err := os.ReadDir(filepath.Join(mnt, d))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if _, err := os.Lstat(filepath.Join(mnt, d, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}

	f, err := os.Open(filepath.Join(mnt, "1"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	bridge := root.EmbeddedInode().bridge
	liveNodes := func() int {
		bridge.ids.mu.Lock()
		defer bridge.ids.mu.Unlock()
		return len(bridge.ids.nodes)
	}
	if got, want := liveNodes(), 203; got != want {
		t.Fatalf("got %d live nodes, want %d", got, want)
	}

	for name := range root.Children() {
		if errno := root.NotifyEntry(name); errno != 0 {
			t.Fatalf("NotifyEntry(%q): %v", name, errno)
		}
	}

	// Left are the root and the open directory; the children of the
	// open directory are not in use.
	want := 2
	deadline := time.Now().Add(5 * time.Second)
	for liveNodes() != want {
		if time.Now().After(deadline) {
			t.Fatalf("got %d live nodes, want %d", liveNodes(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := f.Stat(); err != nil {
		t.Errorf("Stat open directory: %v", err)
	}
}
