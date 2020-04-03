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
}

var _ = (NodeLookuper)((*allChildrenNode)(nil))

func (n *allChildrenNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	stable := StableAttr{
		Mode: syscall.S_IFDIR,
	}

	childFN := &allChildrenNode{}
	child := n.NewInode(ctx, childFN, stable)
	return child, 0
}

func TestForget(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if u.Uid != "0" {
		t.Skip("must run test as root")
	}
	root := &allChildrenNode{}
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
		// we don't shutdown the serve loop. If the mount does
		// not succeed, the loop won't work and exit.
		t.Fatal(err)
	}

	N := 1000
	for i := 0; i < N; i++ {
		nm := ""
		for _, c := range fmt.Sprintf("%d", i) {
			nm += fmt.Sprintf("%c/", c)
		}

		_, err := os.Lstat(filepath.Join(dir, nm))
		if err != nil {
			t.Fatal(err)
		}
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
