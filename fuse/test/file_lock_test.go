// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func TestFlockExclusive(t *testing.T) {
	cmd, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("flock command not found.")
	}
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	tc.WriteFile(tc.origFile, []byte(contents), 0700)

	f, err := os.OpenFile(tc.mountFile, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", tc.mountFile, err)
	}
	defer f.Close()

	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Errorf("Flock returned: %v", err)
		return
	}

	_, err = runExternalFlock(cmd, tc.mountFile)
	if err == nil {
		t.Errorf("Expected flock to fail, but it did not")
	}
}

func runExternalFlock(flockPath, fname string) ([]byte, error) {
	f, err := os.OpenFile(fname, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// in order to test the lock property we must use cmd.ExtraFiles (instead of passing the actual file)
	// if we were to pass the file then this flock command would fail to place the lock (returning a
	// 'file busy' error) as it is already opened and locked at this point (see above)
	cmd := exec.Command(flockPath, "--exclusive", "--nonblock", "3")
	cmd.Env = append(cmd.Env, "LC_ALL=C") // in case the user's shell language is different
	cmd.ExtraFiles = []*os.File{f}
	return cmd.CombinedOutput()
}

type lockingNode struct {
	nodefs.Node

	mu            sync.Mutex
	getLkInvoked  bool
	setLkInvoked  bool
	setLkwInvoked bool
}

func (n *lockingNode) GetLkInvoked() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.getLkInvoked
}

func (n *lockingNode) SetLkInvoked() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.setLkInvoked
}

func (n *lockingNode) SetLkwInvoked() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.setLkwInvoked
}

func (n *lockingNode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	return nodefs.NewDataFile([]byte("hello world")), fuse.OK
}

func (n *lockingNode) GetLk(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock, context *fuse.Context) (code fuse.Status) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.getLkInvoked = true
	return fuse.OK
}

func (n *lockingNode) SetLk(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, context *fuse.Context) (code fuse.Status) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.setLkInvoked = true
	return fuse.OK
}

func (n *lockingNode) SetLkw(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, context *fuse.Context) (code fuse.Status) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.setLkwInvoked = true
	return fuse.OK
}

func TestFlockInvoked(t *testing.T) {
	flock, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("flock command not found.")
	}

	dir := testutil.TempDir()
	defer os.RemoveAll(dir)

	opts := &nodefs.Options{
		Owner: fuse.CurrentOwner(),
		Debug: testutil.VerboseTest(),
	}

	root := nodefs.NewDefaultNode()
	conn := nodefs.NewFileSystemConnector(root, opts)
	mountOpts := fuse.MountOptions{
		EnableLocks: true,
	}
	s, err := fuse.NewServer(conn.RawFS(), dir, &mountOpts)
	if err != nil {
		t.Fatal("NewServer", err)
	}

	go s.Serve()
	if err := s.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	defer s.Unmount()

	node := &lockingNode{
		Node: nodefs.NewDefaultNode(),
	}
	root.Inode().NewChild("foo", false, node)

	realPath := filepath.Join(dir, "foo")

	if node.SetLkInvoked() {
		t.Fatalf("SetLk is invoked")
	}

	cmd := exec.Command(flock, "--nonblock", realPath, "echo", "locked")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("flock %v: %v", err, string(out))
	}
	if !node.SetLkInvoked() {
		t.Fatalf("SetLk is not invoked")
	}

	if node.SetLkwInvoked() {
		t.Fatalf("SetLkw is invoked")
	}
	cmd = exec.Command(flock, realPath, "echo", "locked")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("flock %v: %v", err, string(out))
	}
	if !node.SetLkwInvoked() {
		t.Fatalf("SetLkw is not invoked")
	}
}

// Test that file system that don't implement locking are still
// handled in the VFS layer.
func TestNoLockSupport(t *testing.T) {
	flock, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("flock command not found.")
	}

	tmp, err := ioutil.TempDir("", "TestNoLockSupport")
	if err != nil {
		t.Fatal(err)
	}
	mnt, err := ioutil.TempDir("", "TestNoLockSupport")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmp)
	defer os.RemoveAll(mnt)

	opts := &nodefs.Options{
		Owner: fuse.CurrentOwner(),
		Debug: testutil.VerboseTest(),
	}
	root := nodefs.NewMemNodeFSRoot(tmp)

	lock := fuse.FileLock{}
	outLock := fuse.FileLock{}
	ctx := fuse.Context{}
	if status := root.GetLk(nil, uint64(1), &lock, uint32(0x0), &outLock, &ctx); status != fuse.ENOSYS {
		t.Fatalf("MemNodeFs should not implement locking")
	}

	s, _, err := nodefs.MountRoot(mnt, root, opts)
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	go s.Serve()
	if err := s.WaitMount(); err != nil {
		t.Fatalf("WaitMount: %v", err)
	}
	defer s.Unmount()

	fn := mnt + "/file.txt"
	if err := ioutil.WriteFile(fn, []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command(flock, "--nonblock", fn, "echo", "locked")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("flock %v: %v", err, string(out))
	}
}
