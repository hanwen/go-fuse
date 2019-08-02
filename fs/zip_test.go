// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
)

var testData = map[string]string{
	"file.txt":           "content",
	"dir/subfile1":       "content2",
	"dir/subdir/subfile": "content3",
}

func createZip(data map[string]string) []byte {
	buf := &bytes.Buffer{}

	zw := zip.NewWriter(buf)
	for k, v := range data {
		fw, _ := zw.Create(k)
		fw.Write([]byte(v))
	}

	zw.Close()
	return buf.Bytes()
}

type byteReaderAt struct {
	b []byte
}

func (br *byteReaderAt) ReadAt(data []byte, off int64) (int, error) {
	end := int(off) + len(data)
	if end > len(br.b) {
		end = len(br.b)
	}

	copy(data, br.b[off:end])
	return end - int(off), nil
}

func TestZipFS(t *testing.T) {
	zipBytes := createZip(testData)

	r, err := zip.NewReader(&byteReaderAt{zipBytes}, int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}

	root := &zipRoot{zr: r}
	mntDir, err := ioutil.TempDir("", "ZipFS")
	if err != nil {
		t.Fatal(err)
	}
	server, err := fs.Mount(mntDir, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	for k, v := range testData {
		c, err := ioutil.ReadFile(filepath.Join(mntDir, k))
		if err != nil {
			t.Fatal(err)
		}
		if string(c) != v {
			t.Errorf("got %q, want %q", c, v)
		}
	}

	entries, err := ioutil.ReadDir(mntDir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = e.IsDir()
	}

	want := map[string]bool{
		"dir": true, "file.txt": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestZipFSOnAdd(t *testing.T) {
	zipBytes := createZip(testData)

	r, err := zip.NewReader(&byteReaderAt{zipBytes}, int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}

	zr := &zipRoot{zr: r}

	root := &fs.Inode{}
	mnt, err := ioutil.TempDir("", "ZipFS")
	if err != nil {
		t.Fatal(err)
	}
	server, err := fs.Mount(mnt, root, &fs.Options{
		OnAdd: func(ctx context.Context) {
			root.AddChild("sub",
				root.NewPersistentInode(ctx, zr, fs.StableAttr{Mode: syscall.S_IFDIR}), false)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Unmount()

	c, err := ioutil.ReadFile(mnt + "/sub/dir/subdir/subfile")
	if err != nil {
		t.Fatal("ReadFile", err)
	}
	if got, want := string(c), "content3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
