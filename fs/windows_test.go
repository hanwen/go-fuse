// Copyright 2020 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func TestWindowsEmulations(t *testing.T) {
	mntDir, err := ioutil.TempDir("", "ZipFS")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mntDir)
	origDir, err := ioutil.TempDir("", "ZipFS")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(origDir)

	rootData := &fs.LoopbackRoot{
		NewNode: newWindowsNode,
		Path:    origDir,
	}
	opts := fs.Options{}
	opts.Debug = testutil.VerboseTest()
	server, err := fs.Mount(mntDir, newWindowsNode(rootData, nil, "", nil), &opts)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	data := []byte("hello")
	nm := mntDir + "/file"
	if err := ioutil.WriteFile(nm, data, 0644); err != nil {
		t.Fatal(err)
	}

	if got, err := ioutil.ReadFile(nm); err != nil {
		t.Fatal(err)
	} else if bytes.Compare(got, data) != 0 {
		t.Fatalf("got %q want %q", got, data)
	}

	f, err := os.Open(nm)
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Unlink(nm); err == nil {
		t.Fatal("Unlink should have failed")
	}

	f.Close()
	// Ugh - it may take a while for the RELEASE to be processed.
	time.Sleep(10 * time.Millisecond)

	if err := syscall.Unlink(nm); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
}
