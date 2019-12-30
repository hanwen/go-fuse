// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package posixtest file systems for generic posix conformance.
package posixtest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// All holds a map of all test functions
var All = map[string]func(*testing.T, string){
	"AppendWrite":                AppendWrite,
	"SymlinkReadlink":            SymlinkReadlink,
	"FileBasic":                  FileBasic,
	"TruncateFile":               TruncateFile,
	"TruncateNoFile":             TruncateNoFile,
	"FdLeak":                     FdLeak,
	"MkdirRmdir":                 MkdirRmdir,
	"NlinkZero":                  NlinkZero,
	"FstatDeleted":               FstatDeleted,
	"ParallelFileOpen":           ParallelFileOpen,
	"Link":                       Link,
	"LinkUnlinkRename":           LinkUnlinkRename,
	"RenameOverwriteDestNoExist": RenameOverwriteDestNoExist,
	"RenameOverwriteDestExist":   RenameOverwriteDestExist,
	"RenameOpenDir":              RenameOpenDir,
	"ReadDir":                    ReadDir,
	"ReadDirPicksUpCreate":       ReadDirPicksUpCreate,
	"DirectIO":                   DirectIO,
	"OpenAt":                     OpenAt,
	"Fallocate":                  Fallocate,
	"DirSeek":                    DirSeek,
}

func DirectIO(t *testing.T, mnt string) {
	fn := mnt + "/file.txt"
	fd, err := syscall.Open(fn, syscall.O_TRUNC|syscall.O_CREAT|syscall.O_DIRECT|syscall.O_WRONLY, 0644)
	if err == syscall.EINVAL {
		t.Skip("FS does not support O_DIRECT")
	}
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if fd != 0 {
			syscall.Close(fd)
		}
	}()
	data := bytes.Repeat([]byte("bye"), 4096)
	if n, err := syscall.Write(fd, data); err != nil || n != len(data) {
		t.Fatalf("Write: %v (%d)", err, n)
	}

	err = syscall.Close(fd)
	fd = 0
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	fd, err = syscall.Open(fn, syscall.O_DIRECT|syscall.O_RDONLY, 0644)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}

	roundtrip := bytes.Repeat([]byte("xxx"), 4096)
	if n, err := syscall.Read(fd, roundtrip); err != nil || n != len(data) {
		t.Fatalf("ReadAt: %v (%d)", err, n)
	}

	if bytes.Compare(roundtrip, data) != 0 {
		t.Errorf("roundtrip made changes: got %q.., want %q..", roundtrip[:10], data[:10])
	}
}

// SymlinkReadlink tests basic symlink functionality
func SymlinkReadlink(t *testing.T, mnt string) {
	err := os.Symlink("/foobar", mnt+"/link")
	if err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	val, err := os.Readlink(mnt + "/link")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}

	if val != "/foobar" {
		t.Errorf("symlink mismatch: %v", val)
	}
}

func FileBasic(t *testing.T, mnt string) {
	content := []byte("hello world")
	fn := mnt + "/file"

	if err := ioutil.WriteFile(fn, content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := ioutil.ReadFile(fn); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if bytes.Compare(got, content) != 0 {
		t.Errorf("ReadFile: got %q, want %q", got, content)
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
	if got, want := uint32(stat.Mode), uint32(fuse.S_IFREG|0755); got != want {
		t.Errorf("Fstat: got mode %o, want %o", got, want)
	}

	if err := f.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TruncateFile(t *testing.T, mnt string) {
	content := []byte("hello world")
	fn := mnt + "/file"
	if err := ioutil.WriteFile(fn, content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := os.OpenFile(fn, os.O_RDWR, 0644)
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

	if got, err := ioutil.ReadFile(fn); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if want := content[:trunc]; bytes.Compare(got, want) != 0 {
		t.Errorf("got %q, want %q", got, want)
	}
}
func TruncateNoFile(t *testing.T, mnt string) {
	fn := mnt + "/file"
	if err := ioutil.WriteFile(fn, []byte("hello"), 0644); err != nil {
		t.Errorf("WriteFile: %v", err)
	}

	if err := syscall.Truncate(fn, 1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	var st syscall.Stat_t
	if err := syscall.Lstat(fn, &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if st.Size != 1 {
		t.Errorf("got size %d, want 1", st.Size)
	}
}

func FdLeak(t *testing.T, mnt string) {
	fn := mnt + "/file"

	if err := ioutil.WriteFile(fn, []byte("hello world"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for i := 0; i < 100; i++ {
		if _, err := ioutil.ReadFile(fn); err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
	}

	if runtime.GOOS == "linux" {
		infos := listFds(0, "")
		if len(infos) > 15 {
			t.Errorf("found %d open file descriptors for 100x ReadFile: %v", len(infos), infos)
		}
	}
}

func MkdirRmdir(t *testing.T, mnt string) {
	fn := mnt + "/dir"

	if err := os.Mkdir(fn, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if fi, err := os.Lstat(fn); err != nil {
		t.Fatalf("Lstat %v", err)
	} else if !fi.IsDir() {
		t.Fatalf("is not a directory")
	}

	if err := os.Remove(fn); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func NlinkZero(t *testing.T, mnt string) {
	src := mnt + "/src"
	dst := mnt + "/dst"
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

// FstatDeleted is similar to NlinkZero, but Fstat()s multiple deleted files
// in random order and checks that the results match an earlier Stat().
//
// Excercises the fd-finding logic in rawBridge.GetAttr.
func FstatDeleted(t *testing.T, mnt string) {
	const iMax = 9
	type file struct {
		fd int
		st syscall.Stat_t
	}
	files := make(map[int]file)
	for i := 0; i <= iMax; i++ {
		// Create files with different sizes
		path := fmt.Sprintf("%s/%d", mnt, i)
		content := make([]byte, i)
		err := ioutil.WriteFile(path, content, 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		var st syscall.Stat_t
		err = syscall.Stat(path, &st)
		if err != nil {
			t.Fatal(err)
		}
		// Open
		fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[i] = file{fd, st}
		defer syscall.Close(fd)
		// Delete
		err = syscall.Unlink(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Fstat in random order
	for _, v := range files {
		var st syscall.Stat_t
		err := syscall.Fstat(v.fd, &st)
		if err != nil {
			t.Fatal(err)
		}
		// Ignore ctime, changes on unlink
		v.st.Ctim = syscall.Timespec{}
		st.Ctim = syscall.Timespec{}
		// Nlink value should have dropped to zero
		v.st.Nlink = 0
		// Rest should stay the same
		if v.st != st {
			t.Errorf("stat mismatch: want=%v\n have=%v", v.st, st)
		}
	}
}

func ParallelFileOpen(t *testing.T, mnt string) {
	fn := mnt + "/file"
	if err := ioutil.WriteFile(fn, []byte("content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	N := 10
	errs := make(chan error, N)
	one := func(b byte) {
		f, err := os.OpenFile(fn, os.O_RDWR, 0644)
		if err != nil {
			errs <- err
			return
		}
		var buf [10]byte
		f.Read(buf[:])
		buf[0] = b
		f.WriteAt(buf[0:1], 2)
		f.Close()
		errs <- nil
	}
	for i := 0; i < N; i++ {
		go one(byte(i))
	}

	for i := 0; i < N; i++ {
		if e := <-errs; e != nil {
			t.Error(e)
		}
	}
}

func Link(t *testing.T, mnt string) {
	link := mnt + "/link"
	target := mnt + "/target"

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

	if st.Nlink != 2 {
		t.Errorf("Expect 2 links, got %d", st.Nlink)
	}
}

func RenameOverwriteDestNoExist(t *testing.T, mnt string) {
	RenameOverwrite(t, mnt, false)
}

func RenameOverwriteDestExist(t *testing.T, mnt string) {
	RenameOverwrite(t, mnt, true)
}

func RenameOverwrite(t *testing.T, mnt string, destExists bool) {
	if err := os.Mkdir(mnt+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := ioutil.WriteFile(mnt+"/file", []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if destExists {
		if err := ioutil.WriteFile(mnt+"/dir/renamed", []byte("xx"), 0644); err != nil {
			t.Fatalf("WriteFile dest: %v", err)
		}
	}

	st := syscall.Stat_t{}
	if err := syscall.Lstat(mnt+"/file", &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	}
	beforeIno := st.Ino
	if err := os.Rename(mnt+"/file", mnt+"/dir/renamed"); err != nil {
		t.Errorf("Rename: %v", err)
	}

	if fi, err := os.Lstat(mnt + "/file"); err == nil {
		t.Fatalf("Lstat old: %v", fi)
	}

	if err := syscall.Lstat(mnt+"/dir/renamed", &st); err != nil {
		t.Fatalf("Lstat after: %v", err)
	}

	if got := st.Ino; got != beforeIno {
		t.Errorf("got ino %d, want %d", got, beforeIno)
	}
}

func RenameOpenDir(t *testing.T, mnt string) {
	if err := os.Mkdir(mnt+"/dir1", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Different permissions so directories are easier to tell apart
	if err := os.Mkdir(mnt+"/dir2", 0700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	var st1 syscall.Stat_t
	if err := syscall.Stat(mnt+"/dir2", &st1); err != nil {
		t.Fatalf("Stat: %v", err)
	}

	fd, err := syscall.Open(mnt+"/dir2", syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer syscall.Close(fd)
	if err := syscall.Rename(mnt+"/dir1", mnt+"/dir2"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	var st2 syscall.Stat_t
	if err := syscall.Fstat(fd, &st2); err != nil {
		t.Skipf("Fstat failed: %v. Known limitation - see https://github.com/hanwen/go-fuse/issues/55", err)
	}
	if st2.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		t.Errorf("got mode %o, want %o", st2.Mode, syscall.S_IFDIR)
	}
	if st2.Ino != st1.Ino {
		t.Errorf("got ino %d, want %d", st2.Ino, st1.Ino)
	}
	if st2.Mode&0777 != st1.Mode&0777 {
		t.Skipf("got permissions %#o, want %#o. Known limitation - see https://github.com/hanwen/go-fuse/issues/55",
			st2.Mode&0777, st1.Mode&0777)
	}
}

// ReadDir creates 110 files one by one, checking that we get the expected
// entries after each file creation.
func ReadDir(t *testing.T, mnt string) {
	want := map[string]bool{}
	// 40 bytes of filename, so 110 entries overflows a
	// 4096 page.
	for i := 0; i < 110; i++ {
		nm := fmt.Sprintf("file%036x", i)
		want[nm] = true
		if err := ioutil.WriteFile(filepath.Join(mnt, nm), []byte("hello"), 0644); err != nil {
			t.Fatalf("WriteFile %q: %v", nm, err)
		}
		// Verify that we get the expected entries
		f, err := os.Open(mnt)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		names, err := f.Readdirnames(-1)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		f.Close()
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
}

// Readdir should pick file created after open, but before readdir.
func ReadDirPicksUpCreate(t *testing.T, mnt string) {
	f, err := os.Open(mnt)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := ioutil.WriteFile(mnt+"/file", []byte{42}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	f.Close()

	if len(names) != 1 || names[0] != "file" {
		t.Errorf("missing file created after opendir")
	}
}

// LinkUnlinkRename implements rename with a link/unlink sequence
func LinkUnlinkRename(t *testing.T, mnt string) {
	content := []byte("hello")
	tmp := mnt + "/tmpfile"
	if err := ioutil.WriteFile(tmp, content, 0644); err != nil {
		t.Fatalf("WriteFile %q: %v", tmp, err)
	}

	dest := mnt + "/file"
	if err := syscall.Link(tmp, dest); err != nil {
		t.Fatalf("Link %q %q: %v", tmp, dest, err)
	}
	if err := syscall.Unlink(tmp); err != nil {
		t.Fatalf("Unlink %q: %v", tmp, err)
	}

	if back, err := ioutil.ReadFile(dest); err != nil {
		t.Fatalf("Read %q: %v", dest, err)
	} else if bytes.Compare(back, content) != 0 {
		t.Fatalf("Read got %q want %q", back, content)
	}
}

// test open with O_APPEND
func AppendWrite(t *testing.T, mnt string) {
	fd, err := syscall.Open(mnt+"/file", syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT, 0644)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if fd != 0 {
			syscall.Close(fd)
		}
	}()
	if _, err := syscall.Write(fd, []byte("hello")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}

	if _, err := syscall.Write(fd, []byte("world")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	if err := syscall.Close(fd); err != nil {
		t.Fatalf("Open: %v", err)
	}
	fd = 0
	want := []byte("helloworld")

	got, err := ioutil.ReadFile(mnt + "/file")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if bytes.Compare(got, want) != 0 {
		t.Errorf("got %q want %q", got, want)
	}
}

// OpenAt tests syscall.Openat().
//
// Hint:
// $ go test ./fs -run TestPosix/OpenAt -v
func OpenAt(t *testing.T, mnt string) {
	dir1 := mnt + "/dir1"
	err := os.Mkdir(dir1, 0777)
	if err != nil {
		t.Fatal(err)
	}
	dirfd, err := syscall.Open(dir1, syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(dirfd)
	dir2 := mnt + "/dir2"
	err = os.Rename(dir1, dir2)
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Openat(dirfd, "file1", syscall.O_CREAT, 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	_, err = os.Stat(dir2 + "/file1")
	if err != nil {
		t.Error(err)
	}
}

func Fallocate(t *testing.T, mnt string) {
	rwFile, err := os.OpenFile(mnt+"/file", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer rwFile.Close()
	err = syscall.Fallocate(int(rwFile.Fd()), 0, 1024, 4096)
	if err != nil {
		t.Fatalf("Fallocate failed: %v", err)
	}
	fi, err := os.Lstat(mnt + "/file")
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if fi.Size() < (1024 + 4096) {
		t.Fatalf("fallocate should have changed file size. Got %d bytes",
			fi.Size())
	}
}
