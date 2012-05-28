package fuse

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var _ = strings.Join
var _ = log.Println

////////////////
// state for our testcase, mostly constants

const mode uint32 = 0757

type testCase struct {
	tmpDir string
	orig   string
	mnt    string

	mountFile   string
	mountSubdir string
	origFile    string
	origSubdir  string
	tester      *testing.T
	state       *MountState
	pathFs      *PathNodeFs
	connector   *FileSystemConnector
}

const testTtl = 100 * time.Millisecond

// Create and mount filesystem.
func NewTestCase(t *testing.T) *testCase {
	me := &testCase{}
	me.tester = t
	paranoia = true

	// Make sure system setting does not affect test.
	syscall.Umask(0)

	const name string = "hello.txt"
	const subdir string = "subdir"

	var err error
	me.tmpDir, err = ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	me.orig = me.tmpDir + "/orig"
	me.mnt = me.tmpDir + "/mnt"

	os.Mkdir(me.orig, 0700)
	os.Mkdir(me.mnt, 0700)

	me.mountFile = filepath.Join(me.mnt, name)
	me.mountSubdir = filepath.Join(me.mnt, subdir)
	me.origFile = filepath.Join(me.orig, name)
	me.origSubdir = filepath.Join(me.orig, subdir)

	var pfs FileSystem
	pfs = NewLoopbackFileSystem(me.orig)
	pfs = NewLockingFileSystem(pfs)

	var rfs RawFileSystem
	me.pathFs = NewPathNodeFs(pfs, &PathNodeFsOptions{
		ClientInodes: true})
	me.connector = NewFileSystemConnector(me.pathFs,
		&FileSystemOptions{
			EntryTimeout:    testTtl,
			AttrTimeout:     testTtl,
			NegativeTimeout: 0.0,
		})
	rfs = me.connector
	rfs = NewLockingRawFileSystem(rfs)

	me.connector.Debug = VerboseTest()
	me.state = NewMountState(rfs)
	me.state.Mount(me.mnt, nil)

	me.state.Debug = VerboseTest()

	// Unthreaded, but in background.
	go me.state.Loop()
	return me
}

// Unmount and del.
func (tc *testCase) Cleanup() {
	err := tc.state.Unmount()
	CheckSuccess(err)
	os.RemoveAll(tc.tmpDir)
}

func (tc *testCase) rootNode() *Inode {
	return tc.pathFs.Root().Inode()
}

////////////////
// Tests.

func TestOpenUnreadable(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()
	_, err := os.Open(ts.mnt + "/doesnotexist")
	if err == nil {
		t.Errorf("open non-existent should raise error")
	}
}

func TestTouch(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(ts.origFile, []byte(contents), 0700)
	CheckSuccess(err)
	err = os.Chtimes(ts.mountFile, time.Unix(42, 0), time.Unix(43, 0))
	CheckSuccess(err)

	var stat syscall.Stat_t
	err = syscall.Lstat(ts.mountFile, &stat)
	CheckSuccess(err)
	if stat.Atim.Sec != 42 || stat.Mtim.Sec != 43 {
		t.Errorf("Got wrong timestamps %v", stat)
	}
}

func TestReadThrough(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	content := RandomData(125)
	err := ioutil.WriteFile(ts.origFile, content, 0700)
	CheckSuccess(err)

	err = os.Chmod(ts.mountFile, os.FileMode(mode))
	CheckSuccess(err)

	fi, err := os.Lstat(ts.mountFile)
	CheckSuccess(err)
	if uint32(fi.Mode().Perm()) != mode {
		t.Errorf("Wrong mode %o != %o", int(fi.Mode().Perm()), mode)
	}

	// Open (for read), read.
	f, err := os.Open(ts.mountFile)
	CheckSuccess(err)
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
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)

	err = os.Remove(tc.mountFile)
	CheckSuccess(err)
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
	CheckSuccess(err)
	defer f.Close()

	content := RandomData(125)
	n, err := f.Write(content)
	CheckSuccess(err)
	if n != len(content) {
		t.Errorf("Write mismatch: %v of %v", n, len(content))
	}

	fi, err := os.Lstat(tc.origFile)
	if fi.Mode().Perm() != 0644 {
		t.Errorf("create mode error %o", fi.Mode()&0777)
	}

	f, err = os.Open(tc.origFile)
	CheckSuccess(err)
	defer f.Close()

	var buf [1024]byte
	slice := buf[:]
	n, err = f.Read(slice)
	CheckSuccess(err)
	CompareSlices(t, slice[:n], content)
}

func TestMkdirRmdir(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	// Mkdir/Rmdir.
	err := os.Mkdir(tc.mountSubdir, 0777)
	CheckSuccess(err)
	fi, err := os.Lstat(tc.origSubdir)
	if !fi.IsDir() {
		t.Errorf("Not a directory: %v", fi)
	}

	err = os.Remove(tc.mountSubdir)
	CheckSuccess(err)
}

func TestLinkCreate(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	content := RandomData(125)
	err := ioutil.WriteFile(tc.origFile, content, 0700)
	CheckSuccess(err)
	err = os.Mkdir(tc.origSubdir, 0777)
	CheckSuccess(err)

	// Link.
	mountSubfile := filepath.Join(tc.mountSubdir, "subfile")
	err = os.Link(tc.mountFile, mountSubfile)
	CheckSuccess(err)

	var subStat, stat syscall.Stat_t
	err = syscall.Lstat(mountSubfile, &subStat)
	CheckSuccess(err)
	err = syscall.Lstat(tc.mountFile, &stat)
	CheckSuccess(err)

	if stat.Nlink != 2 {
		t.Errorf("Expect 2 links: %v", stat)
	}
	if stat.Ino != subStat.Ino {
		t.Errorf("Link succeeded, but inode numbers different: %v %v", stat.Ino, subStat.Ino)
	}
	readback, err := ioutil.ReadFile(mountSubfile)
	CheckSuccess(err)
	CompareSlices(t, readback, content)

	err = os.Remove(tc.mountFile)
	CheckSuccess(err)

	_, err = ioutil.ReadFile(mountSubfile)
	CheckSuccess(err)
}

// Deal correctly with hard links implied by matching client inode
// numbers.
func TestLinkExisting(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	c := RandomData(5)

	err := ioutil.WriteFile(tc.orig+"/file1", c, 0644)
	CheckSuccess(err)
	err = os.Link(tc.orig+"/file1", tc.orig+"/file2")
	CheckSuccess(err)

	var s1, s2 syscall.Stat_t
	err = syscall.Lstat(tc.mnt+"/file1", &s1)
	CheckSuccess(err)
	err = syscall.Lstat(tc.mnt+"/file2", &s2)
	CheckSuccess(err)

	if s1.Ino != s2.Ino {
		t.Errorf("linked files should have identical inodes %v %v", s1.Ino, s2.Ino)
	}

	back, err := ioutil.ReadFile(tc.mnt + "/file1")
	CheckSuccess(err)
	CompareSlices(t, back, c)
}

// Deal correctly with hard links implied by matching client inode
// numbers.
func TestLinkForget(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	c := "hello"

	err := ioutil.WriteFile(tc.orig+"/file1", []byte(c), 0644)
	CheckSuccess(err)
	err = os.Link(tc.orig+"/file1", tc.orig+"/file2")
	CheckSuccess(err)

	var s1, s2 syscall.Stat_t
	err = syscall.Lstat(tc.mnt+"/file1", &s1)
	CheckSuccess(err)

	tc.pathFs.ForgetClientInodes()

	err = syscall.Lstat(tc.mnt+"/file2", &s2)
	CheckSuccess(err)
	if s1.Ino == s2.Ino {
		t.Error("After forget, we should not export links")
	}
}

func TestSymlink(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	t.Log("testing symlink/readlink.")
	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)

	linkFile := "symlink-file"
	orig := "hello.txt"
	err = os.Symlink(orig, filepath.Join(tc.mnt, linkFile))

	CheckSuccess(err)

	origLink := filepath.Join(tc.orig, linkFile)
	fi, err := os.Lstat(origLink)
	CheckSuccess(err)

	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("not a symlink: %v", fi)
		return
	}

	read, err := os.Readlink(filepath.Join(tc.mnt, linkFile))
	CheckSuccess(err)

	if read != orig {
		t.Errorf("unexpected symlink value '%v'", read)
	}
}

func TestRename(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)
	sd := tc.mnt + "/testRename"
	err = os.MkdirAll(sd, 0777)

	subFile := sd + "/subfile"
	err = os.Rename(tc.mountFile, subFile)
	CheckSuccess(err)
	f, _ := os.Lstat(tc.origFile)
	if f != nil {
		t.Errorf("original %v still exists.", tc.origFile)
	}
	f, _ = os.Lstat(subFile)
	if f == nil {
		t.Errorf("destination %v does not exist.", subFile)
	}
}

// Flaky test, due to rename race condition.
func TestDelRename(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	t.Log("Testing del+rename.")

	sd := tc.mnt + "/testDelRename"
	err := os.MkdirAll(sd, 0755)
	CheckSuccess(err)

	d := sd + "/dest"
	err = ioutil.WriteFile(d, []byte("blabla"), 0644)
	CheckSuccess(err)

	f, err := os.Open(d)
	CheckSuccess(err)
	defer f.Close()

	err = os.Remove(d)
	CheckSuccess(err)

	s := sd + "/src"
	err = ioutil.WriteFile(s, []byte("blabla"), 0644)
	CheckSuccess(err)

	err = os.Rename(s, d)
	CheckSuccess(err)
}

func TestOverwriteRename(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	t.Log("Testing rename overwrite.")

	sd := tc.mnt + "/testOverwriteRename"
	err := os.MkdirAll(sd, 0755)
	CheckSuccess(err)

	d := sd + "/dest"
	err = ioutil.WriteFile(d, []byte("blabla"), 0644)
	CheckSuccess(err)

	s := sd + "/src"
	err = ioutil.WriteFile(s, []byte("blabla"), 0644)
	CheckSuccess(err)

	err = os.Rename(s, d)
	CheckSuccess(err)
}

func TestAccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Log("Skipping TestAccess() as root.")
		return
	}
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)
	err = os.Chmod(tc.origFile, 0)
	CheckSuccess(err)
	// Ugh - copied from unistd.h
	const W_OK uint32 = 2

	errCode := syscall.Access(tc.mountFile, W_OK)
	if errCode != syscall.EACCES {
		t.Errorf("Expected EACCES for non-writable, %v %v", errCode, syscall.EACCES)
	}
	err = os.Chmod(tc.origFile, 0222)
	CheckSuccess(err)
	errCode = syscall.Access(tc.mountFile, W_OK)
	if errCode != nil {
		t.Errorf("Expected no error code for writable. %v", errCode)
	}
}

func TestMknod(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	t.Log("Testing mknod.")
	errNo := syscall.Mknod(tc.mountFile, syscall.S_IFIFO|0777, 0)
	if errNo != nil {
		t.Errorf("Mknod %v", errNo)
	}
	fi, _ := os.Lstat(tc.origFile)
	if fi == nil || fi.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("Expected FIFO filetype.")
	}
}

func TestReaddir(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)
	err = os.Mkdir(tc.origSubdir, 0777)
	CheckSuccess(err)

	dir, err := os.Open(tc.mnt)
	CheckSuccess(err)
	infos, err := dir.Readdir(10)
	CheckSuccess(err)

	wanted := map[string]bool{
		"hello.txt": true,
		"subdir":    true,
	}
	if len(wanted) != len(infos) {
		t.Errorf("Length mismatch %v", infos)
	} else {
		for _, v := range infos {
			_, ok := wanted[v.Name()]
			if !ok {
				t.Errorf("Unexpected name %v", v.Name())
			}
		}
	}

	dir.Close()
}

func TestFSync(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(tc.origFile, []byte(contents), 0700)
	CheckSuccess(err)

	f, err := os.OpenFile(tc.mountFile, os.O_WRONLY, 0)
	_, err = f.WriteString("hello there")
	CheckSuccess(err)

	// How to really test fsync ?
	err = syscall.Fsync(int(f.Fd()))
	if err != nil {
		t.Errorf("fsync returned: %v", err)
	}
	f.Close()
}

func TestReadZero(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()
	err := ioutil.WriteFile(ts.origFile, []byte{}, 0644)
	CheckSuccess(err)

	back, err := ioutil.ReadFile(ts.mountFile)
	CheckSuccess(err)
	if len(back) != 0 {
		t.Errorf("content length: got %d want %d", len(back), 0)
	}
}

func RandomData(size int) []byte {
	// Make blocks that are not period on 1024 bytes, so we can
	// catch errors due to misalignments.
	block := make([]byte, 1023)
	content := make([]byte, size)
	for i := range block {
		block[i] = byte(i)
	}
	start := 0
	for start < len(content) {
		left := len(content) - start
		if left < len(block) {
			block = block[:left]
		}

		copy(content[start:], block)
		start += len(block)
	}
	return content
}

func CompareSlices(t *testing.T, got, want []byte) {
	if len(got) != len(want) {
		t.Errorf("content length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if i >= len(got) {
			break
		}
		if want[i] != got[i] {
			t.Errorf("content mismatch byte %d, got %d want %d.", i, got[i], want[i])
			break
		}
	}
}

// Check that reading large files doesn't lead to large allocations.
func TestReadLargeMemCheck(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	content := RandomData(385 * 1023)
	err := ioutil.WriteFile(ts.origFile, []byte(content), 0644)
	CheckSuccess(err)

	f, err := os.Open(ts.mountFile)
	CheckSuccess(err)
	buf := make([]byte, len(content)+1024)
	f.Read(buf)
	CheckSuccess(err)
	f.Close()
	runtime.GC()
	var before, after runtime.MemStats

	N := 100
	runtime.ReadMemStats(&before)
	for i := 0; i < N; i++ {
		f, _ := os.Open(ts.mountFile)
		f.Read(buf)
		f.Close()
	}
	runtime.ReadMemStats(&after)
	delta := int((after.TotalAlloc - before.TotalAlloc))
	delta = (delta - 40000) / N

	limit := 5000
	if unsafe.Sizeof(uintptr(0)) == 8 {
		limit = 10000
	}
	if delta > limit {
		t.Errorf("bytes per loop: %d, limit %d", delta, limit)
	}
}

func TestReadLarge(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	content := RandomData(385 * 1023)
	err := ioutil.WriteFile(ts.origFile, []byte(content), 0644)
	CheckSuccess(err)

	back, err := ioutil.ReadFile(ts.mountFile)
	CheckSuccess(err)
	CompareSlices(t, back, content)
}

func randomLengthString(length int) string {
	r := rand.Intn(length)
	j := 0

	b := make([]byte, r)
	for i := 0; i < r; i++ {
		j = (j + 1) % 10
		b[i] = byte(j) + byte('0')
	}
	return string(b)
}

func TestLargeDirRead(t *testing.T) {
	tc := NewTestCase(t)
	defer tc.Cleanup()

	t.Log("Testing large readdir.")
	created := 100

	names := make([]string, created)

	subdir := filepath.Join(tc.orig, "readdirSubdir")
	os.Mkdir(subdir, 0700)
	longname := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	nameSet := make(map[string]bool)
	for i := 0; i < created; i++ {
		// Should vary file name length.
		base := fmt.Sprintf("file%d%s", i,
			randomLengthString(len(longname)))
		name := filepath.Join(subdir, base)

		nameSet[base] = true

		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0777)
		CheckSuccess(err)
		f.WriteString("bla")
		f.Close()

		names[i] = name
	}

	dir, err := os.Open(filepath.Join(tc.mnt, "readdirSubdir"))
	CheckSuccess(err)
	defer dir.Close()

	// Chunked read.
	total := 0
	readSet := make(map[string]bool)
	for {
		namesRead, err := dir.Readdirnames(200)
		if len(namesRead) == 0 || err == io.EOF {
			break
		}
		CheckSuccess(err)
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

func TestRootDir(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	d, err := os.Open(ts.mnt)
	CheckSuccess(err)
	_, err = d.Readdirnames(-1)
	CheckSuccess(err)
	err = d.Close()
	CheckSuccess(err)
}

func TestIoctl(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	f, err := os.OpenFile(filepath.Join(ts.mnt, "hello.txt"),
		os.O_WRONLY|os.O_CREATE, 0777)
	defer f.Close()
	CheckSuccess(err)
	ioctl(int(f.Fd()), 0x5401, 42)
}

func clearStatfs(s *syscall.Statfs_t) {
	empty := syscall.Statfs_t{}
	s.Type = 0
	s.Fsid = empty.Fsid
	s.Spare = empty.Spare
	// TODO - figure out what this is for.
	s.Flags = 0
}

// This test is racy. If an external process consumes space while this
// runs, we may see spurious differences between the two statfs() calls.
func TestStatFs(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	empty := syscall.Statfs_t{}
	s1 := empty
	err := syscall.Statfs(ts.orig, &s1)
	if err != nil {
		t.Fatal("statfs orig", err)
	}

	s2 := syscall.Statfs_t{}
	err = syscall.Statfs(ts.mnt, &s2)

	if err != nil {
		t.Fatal("statfs mnt", err)
	}

	clearStatfs(&s1)
	clearStatfs(&s2)
	if fmt.Sprintf("%v", s2) != fmt.Sprintf("%v", s1) {
		t.Error("Mismatch", s1, s2)
	}
}

func TestFStatFs(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	fOrig, err := os.OpenFile(ts.orig+"/file", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	CheckSuccess(err)
	defer fOrig.Close()

	empty := syscall.Statfs_t{}
	s1 := empty
	errno := syscall.Fstatfs(int(fOrig.Fd()), &s1)
	if errno != nil {
		t.Fatal("statfs orig", err)
	}

	fMnt, err := os.OpenFile(ts.mnt+"/file", os.O_RDWR, 0644)
	CheckSuccess(err)
	defer fMnt.Close()
	s2 := empty

	errno = syscall.Fstatfs(int(fMnt.Fd()), &s2)
	if errno != nil {
		t.Fatal("statfs mnt", err)
	}

	clearStatfs(&s1)
	clearStatfs(&s2)
	if fmt.Sprintf("%v", s2) != fmt.Sprintf("%v", s1) {
		t.Error("Mismatch", s1, s2)
	}
}

func TestOriginalIsSymlink(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(tmpDir)
	orig := tmpDir + "/orig"
	err = os.Mkdir(orig, 0755)
	CheckSuccess(err)
	link := tmpDir + "/link"
	mnt := tmpDir + "/mnt"
	err = os.Mkdir(mnt, 0755)
	CheckSuccess(err)
	err = os.Symlink("orig", link)
	CheckSuccess(err)

	fs := NewLoopbackFileSystem(link)
	nfs := NewPathNodeFs(fs, nil)
	state, _, err := MountNodeFileSystem(mnt, nfs, nil)
	CheckSuccess(err)
	defer state.Unmount()

	go state.Loop()

	_, err = os.Lstat(mnt)
	CheckSuccess(err)
}

func TestDoubleOpen(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	err := ioutil.WriteFile(ts.orig+"/file", []byte("blabla"), 0644)
	CheckSuccess(err)

	roFile, err := os.Open(ts.mnt + "/file")
	CheckSuccess(err)
	defer roFile.Close()

	rwFile, err := os.OpenFile(ts.mnt+"/file", os.O_WRONLY|os.O_TRUNC, 0666)
	CheckSuccess(err)
	defer rwFile.Close()
}

func TestUmask(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	// Make sure system setting does not affect test.
	fn := ts.mnt + "/file"
	mask := 020
	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("umask %o && mkdir %s", mask, fn))
	cmd.Run()

	fi, err := os.Lstat(fn)
	CheckSuccess(err)

	expect := mask ^ 0777
	got := int(fi.Mode().Perm())
	if got != expect {
		t.Errorf("got %o, expect mode %o for file %s", got, expect, fn)
	}
}
