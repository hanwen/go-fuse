// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/internal/testutil"
	"github.com/kylelemons/godebug/pretty"
)

var _ = log.Println

type testCase struct {
	*testing.T

	dir     string
	origDir string
	mntDir  string

	loopback DirOperations
	rawFS    fuse.RawFileSystem
	server   *fuse.Server
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

func newTestCase(t *testing.T, entryCache bool, attrCache bool) *testCase {
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

	tc.loopback = NewLoopback(tc.origDir)
	_ = time.Second

	oneSec := time.Second

	attrDT := &oneSec
	if !attrCache {
		attrDT = nil
	}
	entryDT := &oneSec
	if !entryCache {
		entryDT = nil
	}
	tc.rawFS = NewNodeFS(tc.loopback, &Options{
		Debug:        testutil.VerboseTest(),
		EntryTimeout: entryDT,
		AttrTimeout:  attrDT,
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
	tc := newTestCase(t, true, true)
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
	tc := newTestCase(t, true, true)
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
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	content := []byte("hello world")

	tc.writeOrig("file", string(content), 0755)

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
	tc := newTestCase(t, true, true)
	defer func() {
		if tc != nil {
			tc.Clean()
		}
	}()

	tc.writeOrig("file", "hello world", 0755)

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
	tc := newTestCase(t, true, true)
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
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)

	if destExists {
		tc.writeOrig("/dir/renamed", "xx", 0644)
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
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)
	tc.writeOrig("dir/file", "x", 0644)

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
}

func TestRenameExchange(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)
	tc.writeOrig("dir/file", "x", 0644)

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

	var before1, before2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &before1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &before2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}

	if err := unix.Renameat2(f1, "file", f2, "file", unix.RENAME_EXCHANGE); err != nil {
		t.Errorf("rename EXCHANGE: %v", err)
	}

	var after1, after2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &after1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &after2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	clearCtime := func(s *unix.Stat_t) {
		s.Ctim.Sec = 0
		s.Ctim.Nsec = 0
	}

	clearCtime(&after1)
	clearCtime(&after2)
	clearCtime(&before2)
	clearCtime(&before1)
	if diff := pretty.Compare(after1, before2); diff != "" {
		t.Errorf("after1, before2: %s", diff)
	}
	if !reflect.DeepEqual(after2, before1) {
		t.Errorf("after2, before1: %#v, %#v", after2, before1)
	}
}

func TestNlinkZero(t *testing.T) {
	// xfstest generic/035.
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	src := tc.mntDir + "/src"
	dst := tc.mntDir + "/dst"
	if err := ioutil.WriteFile(src, []byte("source"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := ioutil.WriteFile(dst, []byte("dst"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := syscall.Open(dst, 0, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer syscall.Close(f)

	var st syscall.Stat_t
	if err := syscall.Fstat(f, &st); err != nil {
		t.Errorf("Fstat before: %v", err)
	} else if st.Nlink != 1 {
		t.Errorf("Nlink of file: got %d, want 1", st.Nlink)
	}

	if err := os.Rename(src, dst); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if err := syscall.Fstat(f, &st); err != nil {
		t.Errorf("Fstat after: %v", err)
	} else if st.Nlink != 0 {
		t.Errorf("Nlink of overwritten file: got %d, want 0", st.Nlink)
	}
}

func TestParallelFileOpen(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	fn := tc.mntDir + "/file"
	if err := ioutil.WriteFile(fn, []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var wg sync.WaitGroup
	one := func(b byte) {
		f, err := os.OpenFile(fn, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("OpenFile: %v", err)
		}
		var buf [10]byte
		f.Read(buf[:])
		buf[0] = b
		f.WriteAt(buf[0:1], 2)
		f.Close()
		wg.Done()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go one(byte(i))
	}
	wg.Wait()
}

func TestSymlink(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	fn := tc.mntDir + "/link"
	target := "target"
	if err := os.Symlink(target, fn); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if got, err := os.Readlink(fn); err != nil {
		t.Fatalf("Readlink: %v", err)
	} else if got != target {
		t.Errorf("Readlink: got %q, want %q", got, target)
	}
}

func TestLink(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	link := tc.mntDir + "/link"
	target := tc.mntDir + "/target"

	if err := ioutil.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := syscall.Stat_t{}
	if err := syscall.Lstat(target, &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	}

	beforeIno := st.Ino
	if err := os.Link(target, link); err != nil {
		t.Errorf("Link: %v", err)
	}

	if err := syscall.Lstat(link, &st); err != nil {
		t.Fatalf("Lstat after: %v", err)
	}

	if st.Ino != beforeIno {
		t.Errorf("Lstat after: got %d, want %d", st.Ino, beforeIno)
	}
}

func TestNotifyEntry(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	orig := tc.origDir + "/file"
	fn := tc.mntDir + "/file"
	tc.writeOrig("file", "hello", 0644)

	st := syscall.Stat_t{}
	if err := syscall.Lstat(fn, &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	}

	if err := os.Remove(orig); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	after := syscall.Stat_t{}
	if err := syscall.Lstat(fn, &after); err != nil {
		t.Fatalf("Lstat after: %v", err)
	} else if !reflect.DeepEqual(st, after) {
		t.Fatalf("got after %#v, want %#v", after, st)
	}

	if code := InodeOf(tc.loopback).NotifyEntry("file"); !code.Ok() {
		t.Errorf("notify failed: %v", code)
	}

	if err := syscall.Lstat(fn, &after); err != syscall.ENOENT {
		t.Fatalf("Lstat after: got %v, want ENOENT", err)
	}
}

// XXX Test Notify() , but requires KEEP_CACHE ? or could use mmap?
// XXX Test NotifyDelete?

func TestReadDir(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	f, err := os.Open(tc.mntDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// add entries after opening the directory
	want := map[string]bool{}
	for i := 0; i < 110; i++ {
		// 40 bytes of filename, so 110 entries overflows a
		// 4096 page.
		nm := fmt.Sprintf("file%036x", i)
		want[nm] = true
		tc.writeOrig(nm, "hello", 0644)
	}

	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	got := map[string]bool{}
	for _, e := range names {
		got[e] = true
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d", len(got), len(want))
	}
	for k := range got {
		if !want[k] {
			t.Errorf("got unknown name %q", k)
		}
	}
}

// This test is racy. If an external process consumes space while this
// runs, we may see spurious differences between the two statfs() calls.
func TestStatFs(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	empty := syscall.Statfs_t{}
	orig := empty
	if err := syscall.Statfs(tc.origDir, &orig); err != nil {
		t.Fatal("statfs orig", err)
	}

	mnt := syscall.Statfs_t{}
	if err := syscall.Statfs(tc.mntDir, &mnt); err != nil {
		t.Fatal("statfs mnt", err)
	}

	var mntFuse, origFuse fuse.StatfsOut
	mntFuse.FromStatfsT(&mnt)
	origFuse.FromStatfsT(&orig)

	if !reflect.DeepEqual(mntFuse, origFuse) {
		t.Errorf("Got %#v, want %#v", mntFuse, origFuse)
	}
}

func TestXAttr(t *testing.T) {
	tc := newTestCase(t, true, true)
	defer tc.Clean()

	tc.writeOrig("file", "", 0644)

	buf := make([]byte, 1024)
	attr := "user.xattrtest"
	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err == syscall.ENOTSUP {
		t.Skip("$TMP does not support xattrs. Rerun this test with a $TMPDIR override")
	}

	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err != syscall.ENODATA {
		t.Fatalf("got %v want ENOATTR", err)
	}
	value := []byte("value")
	if err := syscall.Setxattr(tc.mntDir+"/file", attr, value, 0); err != nil {
		t.Fatalf("Setxattr: %v", err)
	}
	sz, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	if bytes.Compare(buf[:sz], value) != 0 {
		t.Fatalf("Getxattr got %q want %q", buf[:sz], value)
	}
	if err := syscall.Removexattr(tc.mntDir+"/file", attr); err != nil {
		t.Fatalf("Removexattr: %v", err)
	}

	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err != syscall.ENODATA {
		t.Fatalf("got %v want ENOATTR", err)
	}
}

func TestGetAttrParallel(t *testing.T) {
	// We grab a file-handle to provide to the API so rename+fstat
	// can be handled correctly. Here, test that closing and
	// (f)stat in parallel don't lead to fstat on closed files.
	// We can only test that if we switch off caching
	tc := newTestCase(t, false, false)
	defer tc.Clean()

	N := 100

	var fds []int
	var fns []string
	for i := 0; i < N; i++ {
		fn := fmt.Sprintf("file%d", i)
		tc.writeOrig(fn, "ello", 0644)
		fn = filepath.Join(tc.mntDir, fn)
		fns = append(fns, fn)
		fd, err := syscall.Open(fn, syscall.O_RDONLY, 0)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}

		fds = append(fds, fd)
	}

	var wg sync.WaitGroup
	wg.Add(2 * N)
	for i := 0; i < N; i++ {
		go func(i int) {
			if err := syscall.Close(fds[i]); err != nil {
				t.Errorf("close %d: %v", i, err)
			}
			wg.Done()
		}(i)
		go func(i int) {
			var st syscall.Stat_t
			if err := syscall.Lstat(fns[i], &st); err != nil {
				t.Errorf("lstat %d: %v", i, err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}
