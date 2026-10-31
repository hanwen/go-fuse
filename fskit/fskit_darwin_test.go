// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package fskit

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestMapMFMountErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rc      int32
		wantErr bool
	}{
		{mfMountResultSuccess, false},
		{mfMountResultUnsupportedOSVersion, true},
		{mfMountResultHelperToolsInstallationFailed, true},
		{mfMountResultFileSystemExtensionNotFound, true},
		{mfMountResultFileSystemExtensionRequiresApproval, true},
		{mfMountResultCreatingMountPointFailed, true},
		{mfMountResultUnexpectedFailure, true},
		{99, true},
	}

	for _, tc := range cases {
		err := mfMountErr(tc.rc)
		if tc.wantErr && err == nil {
			t.Errorf("mfMountErr(%d) = nil, want error", tc.rc)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("mfMountErr(%d) = %v, want nil", tc.rc, err)
		}
	}
}

func TestIsAppleSetXAttr(t *testing.T) {
	t.Parallel()

	buildSetxattrMsg := func(unique uint64, name string) [][]byte {
		hdr := make([]byte, setXAttrHeaderSize+len(name)+1)
		binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(hdr)))
		binary.LittleEndian.PutUint32(hdr[4:8], opSetXAttr)
		binary.LittleEndian.PutUint64(hdr[8:16], unique)
		binary.LittleEndian.PutUint64(hdr[16:24], 1) // nodeid
		copy(hdr[setXAttrHeaderSize:], name)
		return [][]byte{hdr}
	}

	// 1. Apple xattr (e.g. com.apple.provenance) should be detected.
	inApple := buildSetxattrMsg(1001, "com.apple.provenance")
	if !isAppleSetXAttr(inApple) {
		t.Errorf("isAppleSetXAttr(com.apple.provenance) = false, want true")
	}

	// 2. User xattr (e.g. user.mime_type) should NOT be detected.
	inUser := buildSetxattrMsg(1002, "user.mime_type")
	if isAppleSetXAttr(inUser) {
		t.Errorf("isAppleSetXAttr(user.mime_type) = true, want false")
	}
}

func TestFormatOptions(t *testing.T) {
	t.Parallel()

	opts := &fuse.MountOptions{
		AllowOther: true,
		FsName:     "myfs",
		Name:       "mytype",
		MaxWrite:   65536,
		Options:    []string{"ro", "noatime"},
	}
	strs := formatOptions(opts)
	joined := filepath.Join(strs...)
	if !containsStr(strs, "allow_other") {
		t.Errorf("formatOptions missing allow_other: %v", joined)
	}
	if !containsStr(strs, "fsname=myfs") {
		t.Errorf("formatOptions missing fsname: %v", joined)
	}
	if !containsStr(strs, "max_read=65536") {
		t.Errorf("formatOptions missing max_read: %v", joined)
	}
	if !containsStr(strs, "daemon_timeout=0") {
		t.Errorf("formatOptions missing daemon_timeout=0: %v", joined)
	}
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

type fskitTestRoot struct {
	fs.Inode
}

func (r *fskitTestRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(ctx, &fs.MemRegularFile{
		Data: []byte("hello fskit\n"),
	}, fs.StableAttr{Mode: syscall.S_IFREG})
	r.AddChild("hello.txt", ch, false)

	chBig := r.NewPersistentInode(ctx, &fs.MemRegularFile{
		Data: bytes.Repeat([]byte("0123456789abcdef"), 32*1024),
	}, fs.StableAttr{Mode: syscall.S_IFREG})
	r.AddChild("big.bin", chBig, false)
}

func TestFSKitIntegration(t *testing.T) {
	if !Available() {
		t.Skip("FSKit / macFUSE 5.3.0+ not available on this machine")
	}

	mnt, err := os.MkdirTemp("", "fskit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(mnt)

	root := &fskitTestRoot{}
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug: false,
		},
	}

	server, err := MountFS(mnt, root, opts)
	if err != nil {
		t.Skipf("FSKit mount not permitted or failed in this environment: %v", err)
	}
	defer server.Unmount()

	rb := server.responsePool.Get().(*responseBuffer)
	if len(rb.payload) < 1<<20 {
		t.Fatalf("got payload %d, want >= %d", len(rb.payload), 1<<20)
	}

	content, err := os.ReadFile(filepath.Join(mnt, "hello.txt"))
	if err != nil {
		t.Fatalf("reading hello.txt failed: %v", err)
	}
	if string(content) != "hello fskit\n" {
		t.Fatalf("got %q, want %q", string(content), "hello fskit\n")
	}

	big, err := os.ReadFile(filepath.Join(mnt, "big.bin"))
	if err != nil {
		t.Fatalf("reading big.bin failed: %v", err)
	}
	if len(big) != 512*1024 {
		t.Fatalf("got %d, want %d", len(big), 512*1024)
	}
}
