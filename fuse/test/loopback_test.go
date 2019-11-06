// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
)

type testCase struct {
	// Per-testcase temporary directory, usually in /tmp, named something like
	// "$TESTNAME.123456".
	tmpDir string
	// Backing directory. Lives in tmpDir.
	orig string
	// Mountpoint. Lives in tmpDir.
	mnt string

	mountFile   string
	mountSubdir string
	origFile    string
	origSubdir  string
	tester      *testing.T
	state       *fuse.Server
	pathFs      *pathfs.PathNodeFs
	connector   *nodefs.FileSystemConnector
}

const testTTL = 100 * time.Millisecond

// Mkdir is a utility wrapper for os.Mkdir, aborting the test if it fails.
func (tc *testCase) Mkdir(name string, mode os.FileMode) {
	if err := os.Mkdir(name, mode); err != nil {
		tc.tester.Fatalf("Mkdir(%q,%v): %v", name, mode, err)
	}
}

// WriteFile is a utility wrapper for ioutil.WriteFile, aborting the
// test if it fails.
func (tc *testCase) WriteFile(name string, content []byte, mode os.FileMode) {
	if err := ioutil.WriteFile(name, content, mode); err != nil {
		if len(content) > 50 {
			content = append(content[:50], '.', '.', '.')
		}

		tc.tester.Fatalf("WriteFile(%q, %q, %o): %v", name, content, mode, err)
	}
}

// Create and mount filesystem.
func NewTestCase(t *testing.T) *testCase {
	tc := &testCase{}
	tc.tester = t

	// Make sure system setting does not affect test.
	syscall.Umask(0)

	const name string = "hello.txt"
	const subdir string = "subdir"

	var err error
	tc.tmpDir = testutil.TempDir()
	tc.orig = tc.tmpDir + "/orig"
	tc.mnt = tc.tmpDir + "/mnt"

	tc.Mkdir(tc.orig, 0700)
	tc.Mkdir(tc.mnt, 0700)

	tc.mountFile = filepath.Join(tc.mnt, name)
	tc.mountSubdir = filepath.Join(tc.mnt, subdir)
	tc.origFile = filepath.Join(tc.orig, name)
	tc.origSubdir = filepath.Join(tc.orig, subdir)

	var pfs pathfs.FileSystem
	pfs = pathfs.NewLoopbackFileSystem(tc.orig)
	pfs = pathfs.NewLockingFileSystem(pfs)

	tc.pathFs = pathfs.NewPathNodeFs(pfs, &pathfs.PathNodeFsOptions{
		ClientInodes: true})
	tc.connector = nodefs.NewFileSystemConnector(tc.pathFs.Root(),
		&nodefs.Options{
			EntryTimeout:        testTTL,
			AttrTimeout:         testTTL,
			NegativeTimeout:     0.0,
			Debug:               testutil.VerboseTest(),
			LookupKnownChildren: true,
		})
	tc.state, err = fuse.NewServer(
		tc.connector.RawFS(), tc.mnt, &fuse.MountOptions{
			SingleThreaded: true,
			Debug:          testutil.VerboseTest(),
		})
	if err != nil {
		t.Fatal("NewServer:", err)
	}

	go tc.state.Serve()
	if err := tc.state.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	return tc
}

// Unmount and del.
func (tc *testCase) Cleanup() {
	err := tc.state.Unmount()
	if err != nil {
		tc.tester.Fatalf("Unmount failed: %v", err)
	}
	os.RemoveAll(tc.tmpDir)
}

func (tc *testCase) rootNode() *nodefs.Inode {
	return tc.pathFs.Root().Inode()
}

////////////////
// Tests.

func TestOpenUnreadable(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()
	_, err := os.Open(tc.mnt + "/doesnotexist")
	if err == nil {
		t.Errorf("open non-existent should raise error")
	}
}

func TestReadThrough(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := randomData(125)
	tc.WriteFile(tc.origFile, content, 0700)
	var mode uint32 = 0757
	err := os.Chmod(tc.mountFile, os.FileMode(mode))
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	fi, err := os.Lstat(tc.mountFile)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if uint32(fi.Mode().Perm()) != mode {
		t.Errorf("Wrong mode %o != %o", int(fi.Mode().Perm()), mode)
	}

	// Open (for read), read.
	f, err := os.Open(tc.mountFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	var buf [1024]byte
	slice := buf[:]
	n, err := f.Read(slice)
	CompareSlices(t, slice[:n], content)
}

func TestRemove(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	tc.WriteFile(tc.origFile, []byte(contents), 0700)

	err := os.Remove(tc.mountFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	_, err = os.Lstat(tc.origFile)
	if err == nil {
		t.Errorf("Lstat() after delete should have generated error.")
	}
}

func TestWriteThrough(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	// Create (for write), write.
	f, err := os.OpenFile(tc.mountFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	content := randomData(125)
	n, err := f.Write(content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(content) {
		t.Errorf("Write mismatch: %v of %v", n, len(content))
	}

	fi, err := os.Lstat(tc.origFile)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", tc.origFile, err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Errorf("create mode error %o", fi.Mode()&0777)
	}

	f, err = os.Open(tc.origFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	var buf [1024]byte
	slice := buf[:]
	n, err = f.Read(slice)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	CompareSlices(t, slice[:n], content)
}

func TestLinkCreate(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := randomData(125)
	tc.WriteFile(tc.origFile, content, 0700)

	tc.Mkdir(tc.origSubdir, 0777)

	// Link.
	mountSubfile := filepath.Join(tc.mountSubdir, "subfile")
	err := os.Link(tc.mountFile, mountSubfile)
	if err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	var subStat, stat syscall.Stat_t
	err = syscall.Lstat(mountSubfile, &subStat)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	err = syscall.Lstat(tc.mountFile, &stat)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}

	if stat.Nlink != 2 {
		t.Errorf("Expect 2 links: %v", stat)
	}
	if stat.Ino != subStat.Ino {
		t.Errorf("Link succeeded, but inode numbers different: %v %v", stat.Ino, subStat.Ino)
	}
	readback, err := ioutil.ReadFile(mountSubfile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	CompareSlices(t, readback, content)

	err = os.Remove(tc.mountFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err = ioutil.ReadFile(mountSubfile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
}

func randomData(size int) []byte {
	return bytes.Repeat([]byte{'x'}, size)
}

// Deal correctly with hard links implied by matching client inode
// numbers.
func TestLinkExisting(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	c := randomData(5)

	tc.WriteFile(tc.orig+"/file1", c, 0644)

	err := os.Link(tc.orig+"/file1", tc.orig+"/file2")
	if err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	var s1, s2 syscall.Stat_t
	err = syscall.Lstat(tc.mnt+"/file1", &s1)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	err = syscall.Lstat(tc.mnt+"/file2", &s2)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}

	if s1.Ino != s2.Ino {
		t.Errorf("linked files should have identical inodes %v %v", s1.Ino, s2.Ino)
	}

	back, err := ioutil.ReadFile(tc.mnt + "/file1")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	CompareSlices(t, back, c)
}

// Deal correctly with hard links implied by matching client inode
// numbers.
func TestLinkForget(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	c := "hello"

	tc.WriteFile(tc.orig+"/file1", []byte(c), 0644)
	err := os.Link(tc.orig+"/file1", tc.orig+"/file2")
	if err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	for _, fn := range []string{"file1", "file2"} {
		var s syscall.Stat_t
		err = syscall.Lstat(tc.mnt+"/"+fn, &s)
		if err != nil {
			t.Fatalf("Lstat failed: %v", err)
		}
		tc.pathFs.ForgetClientInodes()
	}

	// Now, the backing files are still hardlinked, but go-fuse's
	// view of them should not be because of the
	// ForgetClientInodes call. To prove this, we swap out the
	// files in the backing store, and prove that they are
	// distinct by truncating to different lengths.
	for _, fn := range []string{"file1", "file2"} {
		fn = tc.orig + "/" + fn
		if err := os.Remove(fn); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		tc.WriteFile(fn, []byte(c), 0644)
	}
	for i, fn := range []string{"file1", "file2"} {
		fn = tc.mnt + "/" + fn
		if err := os.Truncate(fn, int64(i)); err != nil {
			t.Fatalf("Truncate: %v", err)
		}
	}

	for i, fn := range []string{"file1", "file2"} {
		var s syscall.Stat_t
		err = syscall.Lstat(tc.mnt+"/"+fn, &s)
		if err != nil {
			t.Fatalf("Lstat failed: %v", err)
		}
		if s.Size != int64(i) {
			t.Errorf("Lstat(%q): got size %d, want %d", fn, s.Size, i)
		}
	}
}

func TestPosix(t *testing.T) {
	tests := []string{
		"SymlinkReadlink",
		"MkdirRmdir",
		"RenameOverwriteDestNoExist",
		"RenameOverwriteDestExist",
		"ReadDir",
		"ReadDirPicksUpCreate",
		"AppendWrite",
	}
	for _, k := range tests {
		f := posixtest.All[k]
		if f == nil {
			t.Fatalf("test %s missing", k)
		}
		t.Run(k, func(t *testing.T) {
			tc := NewTestCase(t)
			defer tc.Cleanup()

			f(t, tc.mnt)
		})
	}
}

// Flaky test, due to rename race condition.
func TestDelRename(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	sd := tc.mnt + "/testDelRename"
	tc.Mkdir(sd, 0755)

	d := sd + "/dest"
	tc.WriteFile(d, []byte("blabla"), 0644)

	f, err := os.Open(d)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	if err := os.Remove(d); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	s := sd + "/src"
	tc.WriteFile(s, []byte("blabla"), 0644)
	if err := os.Rename(s, d); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
}

func TestAccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Log("Skipping TestAccess() as root.")
		return
	}
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	tc.WriteFile(tc.origFile, []byte(contents), 0700)

	if err := os.Chmod(tc.origFile, 0); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	// Ugh - copied from unistd.h
	const W_OK uint32 = 2

	if errCode := syscall.Access(tc.mountFile, W_OK); errCode != syscall.EACCES {
		t.Errorf("Expected EACCES for non-writable, %v %v", errCode, syscall.EACCES)
	}

	if err := os.Chmod(tc.origFile, 0222); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	if errCode := syscall.Access(tc.mountFile, W_OK); errCode != nil {
		t.Errorf("Expected no error code for writable. %v", errCode)
	}
}

func TestMknod(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	if errNo := syscall.Mknod(tc.mountFile, syscall.S_IFIFO|0777, 0); errNo != nil {
		t.Errorf("Mknod %v", errNo)
	}

	if fi, err := os.Lstat(tc.origFile); err != nil {
		t.Errorf("Lstat(%q): %v", tc.origFile, err)
	} else if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("Expected FIFO filetype, got %x", fi.Mode())
	}
}

// Test that READDIR works even if the directory is renamed after the OPENDIR.
// This checks that the fix for https://github.com/hanwen/go-fuse/issues/252
// does not break this case.
func TestReaddirRename(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	tc.Mkdir(tc.origSubdir, 0777)
	tc.WriteFile(tc.origSubdir+"/file.txt", []byte("foo"), 0700)

	dir, err := os.Open(tc.mountSubdir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer dir.Close()

	err = os.Rename(tc.mountSubdir, tc.mountSubdir+".2")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	names, err := dir.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames failed: %v", err)
	}
	if len(names) != 1 || names[0] != "file.txt" {
		t.Fatalf("incorrect directory listing: %v", names)
	}
}

func TestFSync(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	tc.WriteFile(tc.origFile, []byte(contents), 0700)

	f, err := os.OpenFile(tc.mountFile, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", tc.mountFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString("hello there"); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	// How to really test fsync ?
	err = syscall.Fsync(int(f.Fd()))
	if err != nil {
		t.Errorf("fsync returned: %v", err)
	}
}

func TestReadZero(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()
	tc.WriteFile(tc.origFile, []byte{}, 0644)

	back, err := ioutil.ReadFile(tc.mountFile)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", tc.mountFile, err)
	} else if len(back) != 0 {
		t.Errorf("content length: got %d want %d", len(back), 0)
	}
}

func CompareSlices(t *testing.T, got, want []byte) {
	if len(got) != len(want) {
		t.Errorf("content length: got %d want %d", len(got), len(want))
		return
	}

	for i := range want {
		if want[i] != got[i] {
			t.Errorf("content mismatch byte %d, got %d want %d.", i, got[i], want[i])
			break
		}
	}
}

// Check that reading large files doesn't lead to large allocations.
func TestReadLargeMemCheck(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := randomData(385 * 1023)
	tc.WriteFile(tc.origFile, []byte(content), 0644)

	f, err := os.Open(tc.mountFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, len(content)+1024)
	f.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	f.Close()
	runtime.GC()
	var before, after runtime.MemStats

	N := 100
	runtime.ReadMemStats(&before)
	for i := 0; i < N; i++ {
		f, _ := os.Open(tc.mountFile)
		f.Read(buf)
		f.Close()
	}
	runtime.ReadMemStats(&after)
	delta := int((after.TotalAlloc - before.TotalAlloc))
	delta = (delta - 40000) / N
	t.Logf("bytes per read loop: %d", delta)
}

func TestReadLarge(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := randomData(385 * 1023)
	tc.WriteFile(tc.origFile, []byte(content), 0644)

	back, err := ioutil.ReadFile(tc.mountFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	CompareSlices(t, back, content)
}

func TestWriteLarge(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := randomData(385 * 1023)
	tc.WriteFile(tc.mountFile, []byte(content), 0644)

	back, err := ioutil.ReadFile(tc.origFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	CompareSlices(t, back, content)
}

func randomLengthString(length int) string {
	r := rand.Intn(length)

	b := make([]byte, r)
	for i := 0; i < r; i++ {
		b[i] = byte(i%10) + byte('0')
	}
	return string(b)
}

func TestLargeDirRead(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	created := 100

	names := make([]string, created)

	subdir := filepath.Join(tc.orig, "readdirSubdir")

	tc.Mkdir(subdir, 0700)

	longname := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	nameSet := make(map[string]bool)
	for i := 0; i < created; i++ {
		// Should vary file name length.
		base := fmt.Sprintf("file%d%s", i,
			randomLengthString(len(longname)))
		name := filepath.Join(subdir, base)

		nameSet[base] = true

		tc.WriteFile(name, []byte("bla"), 0777)

		names[i] = name
	}

	dir, err := os.Open(filepath.Join(tc.mnt, "readdirSubdir"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer dir.Close()

	// Chunked read.
	total := 0
	readSet := make(map[string]bool)
	for {
		namesRead, err := dir.Readdirnames(200)
		if len(namesRead) == 0 || err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Readdirnames failed: %v", err)
		}
		for _, v := range namesRead {
			readSet[v] = true
		}
		total += len(namesRead)
	}

	if total != created {
		t.Errorf("readdir mismatch got %v wanted %v", total, created)
	}
	for k := range nameSet {
		_, ok := readSet[k]
		if !ok {
			t.Errorf("Name %v not found in output", k)
		}
	}
}

func ioctl(fd int, cmd int, arg uintptr) (int, int) {
	r0, _, e1 := syscall.Syscall(
		syscall.SYS_IOCTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	val := int(r0)
	errno := int(e1)
	return val, errno
}

func TestIoctl(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	f, err := os.OpenFile(filepath.Join(tc.mnt, "hello.txt"),
		os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()
	ioctl(int(f.Fd()), 0x5401, 42)
}

// This test is racy. If an external process consumes space while this
// runs, we may see spurious differences between the two statfs() calls.
func TestNonVerboseStatFs(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	empty := syscall.Statfs_t{}
	s1 := empty
	if err := syscall.Statfs(tc.orig, &s1); err != nil {
		t.Fatal("statfs orig", err)
	}

	s2 := syscall.Statfs_t{}
	if err := syscall.Statfs(tc.mnt, &s2); err != nil {
		t.Fatal("statfs mnt", err)
	}

	clearStatfs(&s1)
	clearStatfs(&s2)
	if !reflect.DeepEqual(s1, s2) {
		t.Errorf("statfs mismatch %#v != %#v", s1, s2)
	}
}

func TestNonVerboseFStatFs(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	fOrig, err := os.OpenFile(tc.orig+"/file", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer fOrig.Close()

	empty := syscall.Statfs_t{}
	s1 := empty
	if errno := syscall.Fstatfs(int(fOrig.Fd()), &s1); errno != nil {
		t.Fatal("statfs orig", err)
	}

	fMnt, err := os.OpenFile(tc.mnt+"/file", os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer fMnt.Close()
	s2 := empty

	if errno := syscall.Fstatfs(int(fMnt.Fd()), &s2); errno != nil {
		t.Fatal("statfs mnt", err)
	}

	clearStatfs(&s1)
	clearStatfs(&s2)
	if !reflect.DeepEqual(s1, s2) {
		t.Errorf("statfs mismatch: %#v != %#v", s1, s2)
	}
}

func TestOriginalIsSymlink(t *testing.T) {
	tmpDir := testutil.TempDir()
	defer os.RemoveAll(tmpDir)
	orig := tmpDir + "/orig"
	err := os.Mkdir(orig, 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	link := tmpDir + "/link"
	mnt := tmpDir + "/mnt"
	if err := os.Mkdir(mnt, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	if err := os.Symlink("orig", link); err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}

	fs := pathfs.NewLoopbackFileSystem(link)
	nfs := pathfs.NewPathNodeFs(fs, nil)
	state, _, err := nodefs.MountRoot(mnt, nfs.Root(), nil)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	defer state.Unmount()

	go state.Serve()
	if err := state.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}

	if _, err := os.Lstat(mnt); err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
}

func TestDoubleOpen(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	tc.WriteFile(tc.orig+"/file", []byte("blabla"), 0644)

	roFile, err := os.Open(tc.mnt + "/file")
	if err != nil {
		t.Fatalf(" failed: %v", err)
	}
	defer roFile.Close()

	rwFile, err := os.OpenFile(tc.mnt+"/file", os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer rwFile.Close()

}

// Check that chgrp(1) works
func TestChgrp(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	f, err := os.Create(tc.mnt + "/file")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	err = f.Chown(-1, os.Getgid())
	if err != nil {
		t.Errorf("Chown failed: %v", err)
	}
}

func TestLookupKnownChildrenAttrCopied(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	if err := ioutil.WriteFile(tc.mountFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fi, err := os.Lstat(tc.mountFile)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mode := fi.Mode()
	time.Sleep(2 * testTTL)

	if fi, err = os.Lstat(tc.mountFile); err != nil {
		t.Fatalf("Lstat: %v", err)
	} else if fi.Mode() != mode {
		t.Fatalf("got mode %o, want %o", fi.Mode(), mode)
	}
}
