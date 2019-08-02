// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unionfs

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type TestFS struct {
	pathfs.FileSystem
	xattrRead int64
}

func (fs *TestFS) GetAttr(path string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	switch path {
	case "":
		return &fuse.Attr{Mode: fuse.S_IFDIR | 0755}, fuse.OK
	case "file":
		return &fuse.Attr{Mode: fuse.S_IFREG | 0755}, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (fs *TestFS) GetXAttr(path string, name string, context *fuse.Context) ([]byte, fuse.Status) {
	if path == "file" && name == "user.attr" {
		atomic.AddInt64(&fs.xattrRead, 1)
		return []byte{42}, fuse.OK
	}
	return nil, fuse.ENOATTR
}

func TestXAttrCaching(t *testing.T) {
	wd := testutil.TempDir()
	defer os.RemoveAll(wd)
	os.Mkdir(wd+"/mnt", 0700)
	err := os.Mkdir(wd+"/rw", 0700)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	rwFS := pathfs.NewLoopbackFileSystem(wd + "/rw")
	roFS := &TestFS{
		FileSystem: pathfs.NewDefaultFileSystem(),
	}

	ufs, err := NewUnionFs([]pathfs.FileSystem{rwFS,
		NewCachingFileSystem(roFS, entryTTL)}, testOpts)
	if err != nil {
		t.Fatalf("NewUnionFs: %v", err)
	}

	opts := &nodefs.Options{
		EntryTimeout:        entryTTL / 2,
		AttrTimeout:         entryTTL / 2,
		NegativeTimeout:     entryTTL / 2,
		Debug:               testutil.VerboseTest(),
		LookupKnownChildren: true,
	}

	pathfs := pathfs.NewPathNodeFs(ufs,
		&pathfs.PathNodeFsOptions{ClientInodes: true,
			Debug: testutil.VerboseTest()})

	server, _, err := nodefs.MountRoot(wd+"/mnt", pathfs.Root(), opts)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	defer server.Unmount()
	go server.Serve()
	server.WaitMount()

	start := time.Now()
	if fi, err := os.Lstat(wd + "/mnt"); err != nil || !fi.IsDir() {
		t.Fatalf("root not readable: %v, %v", err, fi)
	}

	buf := make([]byte, 1024)
	n, err := Getxattr(wd+"/mnt/file", "user.attr", buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	want := "\x2a"
	got := string(buf[:n])
	if got != want {
		t.Fatalf("Got %q want %q", got, err)
	}

	time.Sleep(entryTTL / 3)

	n, err = Getxattr(wd+"/mnt/file", "user.attr", buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	got = string(buf[:n])
	if got != want {
		t.Fatalf("Got %q want %q", got, err)
	}

	time.Sleep(entryTTL / 3)

	// Make sure that an interceding Getxattr() to a filesystem that doesn't implement GetXAttr() doesn't affect future calls.
	Getxattr(wd, "whatever", buf)

	n, err = Getxattr(wd+"/mnt/file", "user.attr", buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	got = string(buf[:n])
	if got != want {
		t.Fatalf("Got %q want %q", got, err)
	}

	if time.Now().Sub(start) >= entryTTL {
		// If we run really slowly, this test will spuriously
		// fail.
		t.Skip("test took too long.")
	}

	actual := atomic.LoadInt64(&roFS.xattrRead)
	if actual != 1 {
		t.Errorf("got xattrRead=%d, want 1", actual)
	}
}
