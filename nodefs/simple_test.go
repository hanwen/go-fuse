// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
)

type testCase struct {
	*testing.T

	dir     string
	origDir string
	mntDir  string

	rawFS  fuse.RawFileSystem
	server *fuse.Server
}

func (tc *testCase) writeOrig(path, content string, mode os.FileMode) {
	if err := ioutil.WriteFile(filepath.Join(tc.origDir, path), []byte(content), mode); err != nil {
		tc.Fatal(err)
	}
}

func (tc *testCase) Clean() {
	if err := tc.server.Unmount(); err != nil {
		tc.Fatal(err)
	}
	if err := os.RemoveAll(tc.dir); err != nil {
		tc.Fatal(err)
	}
}

func newTestCase(t *testing.T) *testCase {
	tc := &testCase{
		dir: testutil.TempDir(),
		T:   t,
	}

	tc.origDir = tc.dir + "/orig"
	tc.mntDir = tc.dir + "/mnt"
	if err := os.Mkdir(tc.origDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tc.mntDir, 0755); err != nil {
		t.Fatal(err)
	}

	loopback := NewLoopback(tc.origDir)
	_ = time.Second
	oneSec := time.Second
	tc.rawFS = NewNodeFS(loopback, &Options{
		Debug: testutil.VerboseTest(),

		// NOSUBMIT - should run all tests without cache too
		EntryTimeout: &oneSec,
		AttrTimeout:  &oneSec,
	})

	var err error
	tc.server, err = fuse.NewServer(tc.rawFS, tc.mntDir,
		&fuse.MountOptions{
			Debug: testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal(err)
	}

	go tc.server.Serve()
	if err := tc.server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	return tc
}

func TestBasic(t *testing.T) {
	tc := newTestCase(t)
	defer tc.Clean()

	tc.writeOrig("file", "hello", 0644)

	fn := tc.mntDir + "/file"
	fi, err := os.Lstat(fn)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if fi.Size() != 5 {
		t.Errorf("got size %d want 5", fi.Size())
	}

	stat := fuse.ToStatT(fi)
	if got, want := stat.Mode, uint32(fuse.S_IFREG|0644); got != want {
		t.Errorf("got mode %o, want %o", got, want)
	}

	if err := os.Remove(fn); err != nil {
		t.Errorf("Remove: %v", err)
	}

	if fi, err := os.Lstat(fn); err == nil {
		t.Errorf("Lstat after remove: got file %v", fi)
	}
}

func TestFile(t *testing.T) {
	tc := newTestCase(t)
	defer tc.Clean()

	content := []byte("hello world")
	fn := tc.mntDir + "/file"

	if err := ioutil.WriteFile(fn, content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := ioutil.ReadFile(fn); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if bytes.Compare(got, content) != 0 {
		t.Errorf("got %q, want %q", got, content)
	}

	f, err := os.Open(fn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer f.Close()

	fi, err := f.Stat()

	if err != nil {
		t.Fatalf("Fstat: %v", err)
	} else if int(fi.Size()) != len(content) {
		t.Errorf("got size %d want 5", fi.Size())
	}

	stat := fuse.ToStatT(fi)
	if got, want := stat.Mode, uint32(fuse.S_IFREG|0755); got != want {
		t.Errorf("Fstat: got mode %o, want %o", got, want)
	}

	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestFileTruncate(t *testing.T) {
	tc := newTestCase(t)
	defer tc.Clean()

	content := []byte("hello world")

	if err := ioutil.WriteFile(tc.origDir+"/file", content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := os.OpenFile(tc.mntDir+"/file", os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	const trunc = 5
	if err := f.Truncate(5); err != nil {
		t.Errorf("Truncate: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	if got, err := ioutil.ReadFile(tc.origDir + "/file"); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if want := content[:trunc]; bytes.Compare(got, want) != 0 {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFileFdLeak(t *testing.T) {
	tc := newTestCase(t)
	defer func() {
		if tc != nil {
			tc.Clean()
		}
	}()

	content := []byte("hello world")

	if err := ioutil.WriteFile(tc.origDir+"/file", content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for i := 0; i < 100; i++ {
		if _, err := ioutil.ReadFile(tc.mntDir + "/file"); err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
	}

	if runtime.GOOS == "linux" {
		infos, err := ioutil.ReadDir("/proc/self/fd")
		if err != nil {
			t.Errorf("ReadDir %v", err)
		}

		if len(infos) > 15 {
			t.Errorf("found %d open file descriptors for 100x ReadFile", len(infos))
		}
	}

	tc.Clean()
	bridge := tc.rawFS.(*rawBridge)
	tc = nil

	if got := len(bridge.files); got > 3 {
		t.Errorf("found %d used file handles, should be <= 3", got)
	}
}

func TestMkdir(t *testing.T) {
	tc := newTestCase(t)
	defer tc.Clean()

	if err := os.Mkdir(tc.mntDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if fi, err := os.Lstat(tc.mntDir + "/dir"); err != nil {
		t.Fatalf("Lstat %v", err)
	} else if !fi.IsDir() {
		t.Fatalf("is not a directory")
	}

	if err := os.Remove(tc.mntDir + "/dir"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func testRenameOverwrite(t *testing.T, destExists bool) {
	tc := newTestCase(t)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := ioutil.WriteFile(tc.origDir+"/file", []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if destExists {
		if err := ioutil.WriteFile(tc.origDir+"/dir/renamed", []byte("xx"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	st := syscall.Stat_t{}
	if err := syscall.Lstat(tc.mntDir+"/file", &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	}
	beforeIno := st.Ino
	if err := os.Rename(tc.mntDir+"/file", tc.mntDir+"/dir/renamed"); err != nil {
		t.Errorf("Rename: %v", err)
	}

	if fi, err := os.Lstat(tc.mntDir + "/file"); err == nil {
		t.Fatalf("Lstat old: %v", fi)
	}

	if err := syscall.Lstat(tc.mntDir+"/dir/renamed", &st); err != nil {
		t.Fatalf("Lstat after: %v", err)
	}

	if got := st.Ino; got != beforeIno {
		t.Errorf("got ino %d, want %d", got, beforeIno)
	}
}

func TestRenameDestExist(t *testing.T) {
	testRenameOverwrite(t, true)
}

func TestRenameDestNoExist(t *testing.T) {
	testRenameOverwrite(t, false)
}

func TestRenameNoOverwrite(t *testing.T) {
	tc := newTestCase(t)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := ioutil.WriteFile(tc.origDir+"/file", []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := ioutil.WriteFile(tc.origDir+"/dir/file", []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f1, err := syscall.Open(tc.mntDir+"/", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	defer syscall.Close(f1)
	f2, err := syscall.Open(tc.mntDir+"/dir", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer syscall.Close(f2)

	if err := unix.Renameat2(f1, "file", f2, "file", unix.RENAME_NOREPLACE); err == nil {
		t.Errorf("rename NOREPLACE succeeded")
	} else if err != syscall.EEXIST {
		t.Errorf("got %v (%T) want EEXIST", err, err)
	}

	if err := unix.Renameat2(f1, "file", f2, "file", unix.RENAME_EXCHANGE); err == nil {
		t.Errorf("rename EXCHANGE succeeded")
	} else if err != syscall.EINVAL {
		t.Errorf("got %v (%T) want %v (%T)", err, err, syscall.EINVAL, syscall.EINVAL)
	}
}
