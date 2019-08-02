// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type umaskFS struct {
	pathfs.FileSystem

	mkdirMode  uint32
	createMode uint32
}

func (fs *umaskFS) Create(name string, flags uint32, mode uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	fs.createMode = mode
	return fs.FileSystem.Create(name, flags, mode, context)
}

func (fs *umaskFS) Mkdir(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fs.mkdirMode = mode
	return fs.FileSystem.Mkdir(name, mode, context)
}

func TestUmask(t *testing.T) {
	tmpDir := testutil.TempDir()
	orig := tmpDir + "/orig"
	mnt := tmpDir + "/mnt"

	os.Mkdir(orig, 0700)
	os.Mkdir(mnt, 0700)

	var pfs pathfs.FileSystem
	pfs = pathfs.NewLoopbackFileSystem(orig)
	pfs = pathfs.NewLockingFileSystem(pfs)
	ufs := &umaskFS{FileSystem: pfs}

	pathFs := pathfs.NewPathNodeFs(ufs, &pathfs.PathNodeFsOptions{
		ClientInodes: true})
	connector := nodefs.NewFileSystemConnector(pathFs.Root(),
		&nodefs.Options{
			EntryTimeout:        testTTL,
			AttrTimeout:         testTTL,
			NegativeTimeout:     0.0,
			Debug:               testutil.VerboseTest(),
			LookupKnownChildren: true,
		})
	server, err := fuse.NewServer(
		connector.RawFS(), mnt, &fuse.MountOptions{
			SingleThreaded: true,
			Debug:          testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal("NewServer:", err)
	}

	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}

	// Make sure system setting does not affect test.
	mask := 020
	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("umask %o && cd %s && mkdir x && touch y", mask, mnt))
	if err := cmd.Run(); err != nil {
		t.Fatalf("cmd.Run: %v", err)
	}

	if err := server.Unmount(); err != nil {
		t.Fatalf("Unmount %v", err)
	}

	if got, want := ufs.mkdirMode&0777, uint32(0757); got != want {
		t.Errorf("got dirMode %o want %o", got, want)
	}
	if got, want := ufs.createMode&0666, uint32(0646); got != want {
		t.Errorf("got createMode %o want %o", got, want)
	}
}
