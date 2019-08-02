// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type cacheFs struct {
	pathfs.FileSystem
}

func (fs *cacheFs) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	f, c := fs.FileSystem.Open(name, flags, context)
	if !c.Ok() {
		return f, c
	}
	return &nodefs.WithFlags{
		File:      f,
		FuseFlags: fuse.FOPEN_KEEP_CACHE,
	}, c

}

func setupCacheTest(t *testing.T) (string, *pathfs.PathNodeFs, func()) {
	dir := testutil.TempDir()
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := &cacheFs{
		pathfs.NewLoopbackFileSystem(dir + "/orig"),
	}
	pfs := pathfs.NewPathNodeFs(fs, &pathfs.PathNodeFsOptions{Debug: testutil.VerboseTest()})

	mntOpts := &fuse.MountOptions{
		// ask kernel not to invalidate file data automatically
		ExplicitDataCacheControl: true,

		Debug: testutil.VerboseTest(),
	}

	opts := nodefs.NewOptions()
	opts.AttrTimeout = 10 * time.Millisecond
	opts.Debug = testutil.VerboseTest()
	state, _, err := nodefs.Mount(dir+"/mnt", pfs.Root(), mntOpts, opts)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	go state.Serve()
	if err := state.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	return dir, pfs, func() {
		err := state.Unmount()
		if err == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestFopenKeepCache(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FOPEN_KEEP_CACHE is broken on Darwin.")
	}

	wd, pathfs, clean := setupCacheTest(t)
	defer clean()

	// x{read,write}File reads/writes file@path and fail on error
	xreadFile := func(path string) string {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	xwriteFile := func(path, data string) {
		if err := ioutil.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// xstat stats path and fails on error
	xstat := func(path string) os.FileInfo {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	// XXX Linux FUSE client automatically invalidates cache of a file if it sees size change.
	//     As workaround we keep len(before) == len(after) to avoid that codepath.
	//     See https://github.com/hanwen/go-fuse/pull/273 for details.
	//
	// TODO use len(before) != len(after) if kernel supports precise control over data cache.
	before := "before"
	after := "afterX"
	if len(before) != len(after) {
		panic("len(before) != len(after)")
	}

	xwriteFile(wd+"/orig/file.txt", before)
	mtimeBefore := xstat(wd + "/orig/file.txt").ModTime()
	c := xreadFile(wd + "/mnt/file.txt")
	if c != before {
		t.Fatalf("ReadFile: got %q, want %q", c, before)
	}

	// sleep a bit to make sure mtime of file for before and after are different
	time.Sleep(20 * time.Millisecond)

	xwriteFile(wd+"/orig/file.txt", after)
	mtimeAfter := xstat(wd + "/orig/file.txt").ModTime()
	if δ := mtimeAfter.Sub(mtimeBefore); δ == 0 {
		panic(fmt.Sprintf("mtime(orig/before) == mtime(orig/after)"))
	}

	// sleep enough time for file attributes to expire; restat the file after.
	// this forces kernel client to relookup/regetattr the file and reread the attributes.
	//
	// this way we make sure the kernel knows updated size/mtime before we
	// try to read the file next time.
	time.Sleep(100 * time.Millisecond)
	_ = xstat(wd + "/mnt/file.txt")

	c = xreadFile(wd + "/mnt/file.txt")
	if c != before {
		t.Fatalf("ReadFile: got %q, want cached %q", c, before)
	}

	if minor := pathfs.Connector().Server().KernelSettings().Minor; minor < 12 {
		t.Skipf("protocol v%d has no notify support.", minor)
	}

	code := pathfs.EntryNotify("", "file.txt")
	if !code.Ok() {
		t.Errorf("EntryNotify: %v", code)
	}

	c = xreadFile(wd + "/mnt/file.txt")
	if c != after {
		t.Fatalf("ReadFile: got %q after notify, want %q", c, after)
	}
}

type nonseekFs struct {
	pathfs.FileSystem
	Length int
}

func (fs *nonseekFs) GetAttr(name string, context *fuse.Context) (fi *fuse.Attr, status fuse.Status) {
	if name == "file" {
		return &fuse.Attr{Mode: fuse.S_IFREG | 0644}, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (fs *nonseekFs) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	if name != "file" {
		return nil, fuse.ENOENT
	}

	data := bytes.Repeat([]byte{42}, fs.Length)
	f := nodefs.NewDataFile(data)
	return &nodefs.WithFlags{
		File:      f,
		FuseFlags: fuse.FOPEN_NONSEEKABLE,
	}, fuse.OK
}

func TestNonseekable(t *testing.T) {
	fs := &nonseekFs{FileSystem: pathfs.NewDefaultFileSystem()}
	fs.Length = 200 * 1024

	dir := testutil.TempDir()
	defer os.RemoveAll(dir)
	nfs := pathfs.NewPathNodeFs(fs, nil)
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	state, _, err := nodefs.MountRoot(dir, nfs.Root(), opts)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer state.Unmount()

	go state.Serve()
	if err := state.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}

	f, err := os.Open(dir + "/file")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer f.Close()

	b := make([]byte, 200)
	n, err := f.ReadAt(b, 20)
	if err == nil || n > 0 {
		t.Errorf("file was opened nonseekable, but seek successful")
	}
}

func TestGetAttrRace(t *testing.T) {
	dir := testutil.TempDir()
	defer os.RemoveAll(dir)
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := pathfs.NewLoopbackFileSystem(dir + "/orig")
	pfs := pathfs.NewPathNodeFs(fs, &pathfs.PathNodeFsOptions{Debug: testutil.VerboseTest()})
	state, _, err := nodefs.MountRoot(dir+"/mnt", pfs.Root(),
		&nodefs.Options{Debug: testutil.VerboseTest()})
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	go state.Serve()
	if err := state.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}

	defer state.Unmount()

	var wg sync.WaitGroup

	n := 100
	wg.Add(n)
	var statErr error
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			fn := dir + "/mnt/file"
			err := ioutil.WriteFile(fn, []byte{42}, 0644)
			if err != nil {
				statErr = err
				return
			}
			_, err = os.Lstat(fn)
			if err != nil {
				statErr = err
			}
		}()
	}
	wg.Wait()
	if statErr != nil {
		t.Error(statErr)
	}
}
