// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"io/ioutil"
	"os"
	"sync/atomic"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/internal/testutil"
)

// exercise functionality when open returns 0 file handle.

// NoFileNode is DataNode for which open returns File=nil.
//
// In other words all operations on a file opened from NoFileNode, are routed
// to NoFileNode itself.
type NoFileNode struct {
	*DataNode

	flags uint32 // FUSE flags for open, e.g. FOPEN_KEEP_CACHE
	nopen int32  // #Open called on us
}

// NewNoFileNode creates new file node for which Open will return File=nil, and
// if flags !=0, will wrap it into WithFlags.
func NewNoFileNode(data []byte, flags uint32) *NoFileNode {
	return &NoFileNode{
		DataNode: NewDataNode(data),
		flags:    flags,
		nopen:    0,
	}
}

func (d *NoFileNode) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	var f nodefs.File = nil
	if d.flags != 0 {
		f = &nodefs.WithFlags{
			File:      f,
			FuseFlags: d.flags,
		}
	}
	atomic.AddInt32(&d.nopen, +1)
	return f, fuse.OK
}

func TestNoFile(t *testing.T) {
	dir := testutil.TempDir()
	defer func() {
		err := os.Remove(dir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	// setup a filesystem with 2 files:
	//
	//	open(hello.txt)	-> nil
	//	open(world.txt)	-> WithFlags(nil, FOPEN_KEEP_CACHE)
	root := nodefs.NewDefaultNode()
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	srv, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}

	hello := NewNoFileNode([]byte("hello"), 0)
	world := NewNoFileNode([]byte("world"), fuse.FOPEN_KEEP_CACHE)
	root.Inode().NewChild("hello.txt", false, hello)
	root.Inode().NewChild("world.txt", false, world)

	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	defer func() {
		err := srv.Unmount()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// assertOpenRead asserts that file @ path reads as dataOK, and that
	// corresponding Open is called on the filesystem.
	assertOpenRead := func(path string, node *NoFileNode, dataOK string) {
		t.Helper()

		nopenPre := atomic.LoadInt32(&node.nopen)

		v, err := ioutil.ReadFile(dir + path)
		if err != nil {
			t.Fatalf("%s: read: %s", path, err)
		}
		if string(v) != dataOK {
			t.Fatalf("%s: read: got %q  ; want %q", path, v, dataOK)
		}

		// make sure that path.Open() was called.
		//
		// this can be not the case if the filesystem has a node with
		// Open that returns ENOSYS - then the kernel won't ever call
		// open for all other nodes.
		nopen := atomic.LoadInt32(&node.nopen)
		if nopen == nopenPre {
			t.Fatalf("%s: read: open was not called", path)
		}
	}

	// make sure all nodes can be open/read.
	assertOpenRead("/hello.txt", hello, "hello")
	assertOpenRead("/world.txt", world, "world")
}
