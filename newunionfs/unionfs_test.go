// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unionfs

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
)

type testCase struct {
	dir    string
	mnt    string
	server *fuse.Server
	rw     string
	ro     string
	root   *unionFSRoot
}

func (tc *testCase) Clean() {
	if tc.server != nil {
		tc.server.Unmount()
		tc.server = nil
	}
	os.RemoveAll(tc.dir)
}

func newTestCase(t *testing.T, populate bool) *testCase {
	t.Helper()
	dir := testutil.TempDir()
	dirs := []string{"ro", "rw", "mnt"}
	if populate {
		dirs = append(dirs, "ro/dir")
	}

	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal("Mkdir", err)
		}
	}

	opts := fs.Options{}
	opts.Debug = testutil.VerboseTest()
	tc := &testCase{
		dir: dir,
		mnt: dir + "/mnt",
		rw:  dir + "/rw",
		ro:  dir + "/ro",
	}
	tc.root = &unionFSRoot{
		roots: []string{tc.rw, tc.ro},
	}

	server, err := fs.Mount(tc.mnt, tc.root, &opts)
	if err != nil {
		t.Fatal("Mount", err)
	}

	tc.server = server

	if populate {
		if err := ioutil.WriteFile(tc.ro+"/dir/ro-file", []byte("bla"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return tc
}

func TestBasic(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	if fi, err := os.Lstat(tc.mnt + "/dir/ro-file"); err != nil {
		t.Fatal(err)
	} else if fi.Size() != 3 {
		t.Errorf("got size %d, want 3", fi.Size())
	}
}

func TestDelete(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	if err := os.Remove(tc.mnt + "/dir/ro-file"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(tc.ro + "/dir/ro-file"); err != nil {
		t.Fatal(err)
	}

	c, err := ioutil.ReadFile(filepath.Join(tc.rw, delDir, filePathHash("dir/ro-file")))
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(c), "dir/ro-file"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestDeleteMarker(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	path := "dir/ro-file"
	tc.root.delPath(path)

	var st syscall.Stat_t
	if err := syscall.Lstat(filepath.Join(tc.mnt, path), &st); err != syscall.ENOENT {
		t.Fatalf("Lstat before: %v", err)
	}

	if errno := tc.root.rmMarker(path); errno != 0 {
		t.Fatalf("rmMarker: %v", errno)
	}

	if err := syscall.Lstat(filepath.Join(tc.mnt, path), &st); err != nil {
		t.Fatalf("Lstat after: %v", err)
	}
}

func TestCreateDeletions(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	if _, err := syscall.Creat(filepath.Join(tc.mnt, delDir), 0644); err != syscall.EPERM {
		t.Fatalf("got err %v, want EPERM", err)
	}
}

func TestCreate(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	path := "dir/ro-file"

	if err := syscall.Unlink(filepath.Join(tc.mnt, path)); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	want := []byte{42}
	if err := ioutil.WriteFile(filepath.Join(tc.mnt, path), want, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := ioutil.ReadFile(filepath.Join(tc.mnt, path)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	} else if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromote(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	path := "dir/ro-file"
	mPath := filepath.Join(tc.mnt, path)

	want := []byte{42}
	if err := ioutil.WriteFile(mPath, want, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := ioutil.ReadFile(mPath); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeleteRevert(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	path := "dir/ro-file"
	mPath := filepath.Join(tc.mnt, path)
	if err := ioutil.WriteFile(mPath, []byte{42}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var st syscall.Stat_t
	if err := syscall.Lstat(mPath, &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	} else if st.Size != 1 {
		t.Fatalf("Stat: want size 1, got %#v", st)
	}

	if err := syscall.Unlink(mPath); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if err := syscall.Lstat(mPath, &st); err != syscall.ENOENT {
		t.Fatalf("Lstat after: got %v, want ENOENT", err)
	}
}

func TestReaddirRoot(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	if err := os.Remove(tc.mnt + "/dir/ro-file"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	f, err := os.Open(tc.mnt)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}

	got := map[string]bool{}
	want := map[string]bool{"dir": true}
	for _, nm := range names {
		got[nm] = true
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestReaddir(t *testing.T) {
	tc := newTestCase(t, true)
	defer tc.Clean()

	if err := ioutil.WriteFile(tc.ro+"/dir/file2", nil, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Mkdir(tc.rw+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := ioutil.WriteFile(tc.rw+"/dir/file3", nil, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Remove(tc.mnt + "/dir/ro-file"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	res, err := ioutil.ReadDir(tc.mnt + "/dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	got := map[string]bool{}
	want := map[string]bool{
		"file2": true,
		"file3": true,
	}
	for _, fi := range res {
		got[fi.Name()] = true
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestPosix(t *testing.T) {
	cases := []string{
		"SymlinkReadlink",
		"FileBasic",
		"TruncateFile",
		"TruncateNoFile",
		"FdLeak",
		//		"MkdirRmdir",
		//		"NlinkZero",
		"ParallelFileOpen",
		//		"Link",
		"ReadDir",
	}

	for _, nm := range cases {
		f := posixtest.All[nm]
		t.Run(nm, func(t *testing.T) {
			tc := newTestCase(t, false)
			defer tc.Clean()

			f(t, tc.mnt)
		})
	}
}

func init() {
	syscall.Umask(0)
}
