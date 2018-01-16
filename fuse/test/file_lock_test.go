// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package test

import (
	"bytes"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"path/filepath"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/internal/testutil"
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

	if out, err := runExternalFlock(cmd, tc.mountFile); !bytes.Contains(out, []byte("failed to get lock")) {
		t.Errorf("runExternalFlock(%q): %s (%v)", tc.mountFile, out, err)
	}
}

func runExternalFlock(flockPath, fname string) ([]byte, error) {
	f, err := os.OpenFile(fname, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cmd := exec.Command(flockPath, "--verbose", "--exclusive", "--nonblock", "3")
	cmd.Env = append(cmd.Env, "LC_ALL=C") // in case the user's shell language is different
	cmd.ExtraFiles = []*os.File{f}
	return cmd.CombinedOutput()
}

type lockingNode struct {
	nodefs.Node
	flockInvoked bool
}

func (n *lockingNode) Open(flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	return nodefs.NewDataFile([]byte("hello world")), fuse.OK
}

func (n *lockingNode) GetLk(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock, context *fuse.Context) (code fuse.Status) {
	n.flockInvoked = true
	return fuse.OK
}

func (n *lockingNode) SetLk(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, context *fuse.Context) (code fuse.Status) {
	n.flockInvoked = true
	return fuse.OK
}

func (n *lockingNode) SetLkw(file nodefs.File, owner uint64, lk *fuse.FileLock, flags uint32, context *fuse.Context) (code fuse.Status) {
	n.flockInvoked = true
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
	s, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	go s.Serve()
	if err := s.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	defer s.Unmount()

	node := &lockingNode{
		Node:    nodefs.NewDefaultNode(),
		flockInvoked: false,
	}
	root.Inode().NewChild("foo", false, node)

	realPath := filepath.Join(dir, "foo")
	cmd:=exec.Command(flock, "--nonblock", realPath, "echo", "locked")
	out, err :=cmd.CombinedOutput()
	if err!=nil {
		t.Fatalf("flock %v: %v",err, string(out))
	}
	if !node.flockInvoked {
		t.Fatalf("flock is not invoked")
	}
}
