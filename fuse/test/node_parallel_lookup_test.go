// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

// verify that several lookup requests can be served in parallel without deadlock.

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"golang.org/x/sync/errgroup"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// tRoot implements simple root node which Lookups children in predefined .nodes.
// The Lookup is synchronized with main test driver on .lookupq and .lookupGo.
type tRoot struct {
	nodefs.Node
	nodes map[string]nodefs.Node // name -> Node

	lookupq  chan string   // main <- fssrv: lookup(name) request received
	lookupGo chan struct{} // main -> fssrv: ok to further process lookup requests
}

func (r *tRoot) Lookup(out *fuse.Attr, name string, fctx *fuse.Context) (*nodefs.Inode, fuse.Status) {
	node, ok := r.nodes[name]
	if !ok {
		// e.g. it can be lookup for .Trash automatically issued by volume manager
		return nil, fuse.ENOENT
	}

	r.lookupq <- name // tell main driver that we received lookup(name)
	<-r.lookupGo      // wait for main to allow us to continue

	st := node.GetAttr(out, nil, fctx)
	return node.Inode(), st
}


// verifyFileRead verifies that file @path has content == dataOK.
func verifyFileRead(path string, dataOK string) error {
	v, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if string(v) != dataOK {
		return fmt.Errorf("%s: file read: got %q  ; want %q", path, v, dataOK)
	}
	return nil
}

func TestNodeParallelLookup(t *testing.T) {
	dir := testutil.TempDir()
	defer func() {
		err := os.Remove(dir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	root := &tRoot{
		Node:     nodefs.NewDefaultNode(),
		nodes:    make(map[string]nodefs.Node),
		lookupq:  make(chan string),
		lookupGo: make(chan struct{}),
	}

	opts := nodefs.NewOptions()
	opts.LookupKnownChildren = true
	opts.Debug = testutil.VerboseTest()
	srv, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}

	root.nodes["hello"] = NewDataNode([]byte("abc"))
	root.nodes["world"] = NewDataNode([]byte("def"))
	root.Inode().NewChild("hello", false, root.nodes["hello"])
	root.Inode().NewChild("world", false, root.nodes["world"])

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

	// the test will deadlock if the client cannot issue several lookups simultaneously
	if srv.KernelSettings().Flags & fuse.CAP_PARALLEL_DIROPS == 0 {
		t.Skip("Kernel serializes dir lookups")
	}

	// spawn 2 threads to access the files in parallel
	// this will deadlock if nodefs does not allow simultaneous Lookups to be handled.
	// see https://github.com/hanwen/go-fuse/commit/d0fca860 for context.
	ctx0, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg, ctx := errgroup.WithContext(ctx0)
	wg.Go(func() error {
		return verifyFileRead(dir + "/hello", "abc")
	})
	wg.Go(func() error {
		return verifyFileRead(dir + "/world", "def")
	})

	// wait till both threads queue into Lookup
	expect := map[string]struct{}{ // set of expected lookups
		"hello": struct{}{},
		"world": struct{}{},
	}
loop:
	for len(expect) > 0 {
		var lookup string
		select {
		case <-ctx.Done():
			break loop // wg.Wait will return the error
		case lookup = <-root.lookupq:
			// ok
		}

		if testutil.VerboseTest() {
			log.Printf("I: <- lookup %q", lookup)
		}
		_, ok := expect[lookup]
		if !ok {
			t.Fatalf("unexpected lookup: %q  ; expect: %q", lookup, expect)
		}
		delete(expect, lookup)
	}

	// let both lookups continue
	close(root.lookupGo)

	err = wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
