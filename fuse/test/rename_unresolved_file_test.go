// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type noopFile struct {
	nodefs.Node
}

func (f *noopFile) GetAttr(out *fuse.Attr, file nodefs.File, conetext *fuse.Context) fuse.Status {
	out.Mode = fuse.S_IFREG | 0777
	out.Size = 123
	return fuse.OK
}

type singleFileDirectory struct {
	nodefs.Node
	currentFilename string
	childNode       nodefs.Node
}

func (d *singleFileDirectory) Open(flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	return nil, fuse.OK
}

func (d *singleFileDirectory) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*nodefs.Inode, fuse.Status) {
	if name != d.currentFilename {
		return nil, fuse.ENOENT
	}
	d.childNode.GetAttr(out, nil, context)
	return d.Inode().NewChild(name, false, d.childNode), fuse.OK
}

func (d *singleFileDirectory) Rename(oldName string, newParent nodefs.Node, newName string, context *fuse.Context) fuse.Status {
	if d != newParent {
		return fuse.EXDEV
	}
	if oldName != d.currentFilename {
		return fuse.ENOENT
	}
	// Lazy implementation that doesn't bother re-adding the child
	// under the new name. Another lookup should cause it to be
	// accessible once again.
	d.Inode().RmChild(oldName)
	d.currentFilename = newName
	return fuse.OK
}

// TestRenameUnresolvedFile renames a file in a directory a couple of
// times in a row. The directory has been implemented in such a way that
// it only removes the old name from the file system. A successive
// lookup should be performed to access the file at the new location.
func TestRenameUnresolvedFile(t *testing.T) {
	dir := testutil.TempDir()
	defer func() {
		err := os.Remove(dir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	root := &singleFileDirectory{
		Node:            nodefs.NewDefaultNode(),
		currentFilename: "foo",
		childNode: &noopFile{
			Node: nodefs.NewDefaultNode(),
		},
	}
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	srv, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}

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

	if err := os.Rename(dir+"/foo", dir+"/bar"); err != nil {
		t.Fatalf("rename: %s", err)
	}
	if err := os.Rename(dir+"/bar", dir+"/baz"); err != nil {
		t.Fatalf("rename: %s", err)
	}
	if err := os.Rename(dir+"/baz", dir+"/qux"); err != nil {
		t.Fatalf("rename: %s", err)
	}
}
