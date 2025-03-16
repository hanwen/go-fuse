// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package posixtest file systems for generic posix conformance.
package posixtest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/fallocate"
	"github.com/hanwen/go-fuse/v2/internal/xattr"
	"golang.org/x/sys/unix"
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
	"LseekHoleSeeksToEOF":        LseekHoleSeeksToEOF,
	"LseekEnxioCheck":            LseekEnxioCheck,
	"RenameOverwriteDestNoExist": RenameOverwriteDestNoExist,
	"RenameOverwriteDestExist":   RenameOverwriteDestExist,
	"RenameOpenDir":              RenameOpenDir,
	"ReadDir":                    ReadDir,
	"ReadDirConsistency":         ReadDirConsistency,
	"DirectIO":                   DirectIO,
	"OpenAt":                     OpenAt,
	"Fallocate":                  Fallocate,
	"DirSeek":                    DirSeek,
	"FcntlFlockSetLk":            FcntlFlockSetLk,
	"FcntlFlockLocksFile":        FcntlFlockLocksFile,
	"SetattrSymlink":             SetattrSymlink,
	"XAttr":                      XAttr,
	"OpenSymlinkRace":            OpenSymlinkRace,
}

func SetattrSymlink(t *testing.T, mnt string) {
	l := filepath.Join(mnt, "link")
	if err := os.Symlink("doesnotexist", l); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	tvs := []unix.Timeval{
		{Sec: 42, Usec: 1},
		{Sec: 43, Usec: 2},
	}
	if err := unix.Lutimes(l, tvs); err != nil {
		t.Fatalf("Lutimes: %v", err)
	}

	var st unix.Stat_t
	if err := unix.Lstat(l, &st); err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if st.Mtim.Sec != 43 {
		// Can't check atime; it's hard to prevent implicit readlink calls.
		t.Fatalf("got mtime %v, want 43", st.Mtim)
	}
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

	if err := os.WriteFile(fn, content, 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := os.ReadFile(fn); err != nil {
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
	if err := os.WriteFile(fn, content, 0755); err != nil {
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

	if got, err := os.ReadFile(fn); err != nil {
		t.Fatalf("ReadFile: %v", err)
	} else if want := content[:trunc]; bytes.Compare(got, want) != 0 {
		t.Errorf("got %q, want %q", got, want)
	}
}
func TruncateNoFile(t *testing.T, mnt string) {
	fn := mnt + "/file"
	if err := os.WriteFile(fn, []byte("hello"), 0644); err != nil {
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

	if err := os.WriteFile(fn, []byte("hello world"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for i := 0; i < 100; i++ {
		if _, err := os.ReadFile(fn); err != nil {
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
	if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := os.WriteFile(dst, []byte("dst"), 0644); err != nil {
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
		st unix.Stat_t
	}
	files := make(map[int]file)
	for i := 0; i <= iMax; i++ {
		// Create files with different sizes
		path := fmt.Sprintf("%s/%d", mnt, i)
		content := make([]byte, i)
		err := os.WriteFile(path, content, 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		var st unix.Stat_t
		err = unix.Stat(path, &st)
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
		var st unix.Stat_t
		err := unix.Fstat(v.fd, &st)
		if err != nil {
			t.Fatal(err)
		}
		// Ignore ctime, changes on unlink
		v.st.Ctim = unix.Timespec{}
		st.Ctim = unix.Timespec{}
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
	if err := os.WriteFile(fn, []byte("content"), 0644); err != nil {
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

	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
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
	if err := os.WriteFile(mnt+"/file", []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if destExists {
		if err := os.WriteFile(mnt+"/dir/renamed", []byte("xx"), 0644); err != nil {
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

func readAllDirEntries(fd int) ([]fuse.DirEntry, error) {
	var buf [4096]byte
	var result []fuse.DirEntry
	for {
		n, err := unix.ReadDirent(fd, buf[:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		todo := buf[:n]
		for len(todo) > 0 {
			var de fuse.DirEntry
			n := de.Parse(todo)
			todo = todo[n:]
			result = append(result, de)
		}
	}
	return result, nil
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
		if err := os.WriteFile(filepath.Join(mnt, nm), []byte("hello"), 0644); err != nil {
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
			t.Errorf("mismatch got %d want %d", len(got), len(want))
		}
		for k := range got {
			if !want[k] {
				t.Errorf("got extra entry %q", k)
			}
		}
		for k := range want {
			if !got[k] {
				t.Errorf("missing entry %q", k)
			}
		}
	}
}

// Creates files, read them back using readdir
func ReadDirConsistency(t *testing.T, mnt string) {
	var results [][]fuse.DirEntry
	for i := 0; i < 2; i++ {
		fd, err := syscall.Open(mnt, syscall.O_DIRECTORY, 0)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer syscall.Close(fd)

		r, err := readAllDirEntries(fd)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		results = append(results, r)
	}
	if !reflect.DeepEqual(results[0], results[1]) {
		t.Errorf("got %v, want %v", results[0], results[1])
	}
}

// LinkUnlinkRename implements rename with a link/unlink sequence
func LinkUnlinkRename(t *testing.T, mnt string) {
	content := []byte("hello")
	tmp := mnt + "/tmpfile"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatalf("WriteFile %q: %v", tmp, err)
	}

	dest := mnt + "/file"
	if err := syscall.Link(tmp, dest); err != nil {
		t.Fatalf("Link %q %q: %v", tmp, dest, err)
	}
	if err := syscall.Unlink(tmp); err != nil {
		t.Fatalf("Unlink %q: %v", tmp, err)
	}

	if back, err := os.ReadFile(dest); err != nil {
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

	got, err := os.ReadFile(mnt + "/file")
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
	fd, err := unix.Openat(dirfd, "file1", syscall.O_CREAT, 0700)
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
	err = fallocate.Fallocate(int(rwFile.Fd()), 0, 1024, 4096)
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

func FcntlFlockSetLk(t *testing.T, mnt string) {
	for i, cmd := range []int{syscall.F_SETLK, syscall.F_SETLKW} {
		filename := mnt + fmt.Sprintf("/file%d", i)
		f1, err := os.Create(filename)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f1.Close()
		wlk := syscall.Flock_t{
			Type:  syscall.F_WRLCK,
			Start: 0,
			Len:   0,
		}
		if err := syscall.FcntlFlock(f1.Fd(), cmd, &wlk); err != nil {
			t.Fatalf("FcntlFlock failed: %v", err)
		}

		f2, err := os.OpenFile(filename, os.O_RDWR, 0766)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f2.Close()
		lk := syscall.Flock_t{}
		if err := sysFcntlFlockGetOFDLock(f2.Fd(), &lk); err != nil {
			t.Errorf("FcntlFlock failed: %v", err)
		}
		if lk.Type != syscall.F_WRLCK {
			t.Errorf("got lk.Type=%v, want %v", lk.Type, syscall.F_WRLCK)
		}
	}
}

func FcntlFlockLocksFile(t *testing.T, mnt string) {
	filename := mnt + "/test"
	f1, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f1.Close()
	wlk := syscall.Flock_t{
		Type:  syscall.F_WRLCK,
		Start: 0,
		Len:   0,
	}
	if err := syscall.FcntlFlock(f1.Fd(), syscall.F_SETLK, &wlk); err != nil {
		t.Fatalf("FcntlFlock failed: %v", err)
	}

	f2, err := os.OpenFile(filename, os.O_RDWR, 0766)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f2.Close()
	rlk := syscall.Flock_t{
		Type:  syscall.F_RDLCK,
		Start: 0,
		Len:   0,
	}
	if err := syscall.FcntlFlock(f2.Fd(), syscall.F_SETLK, &rlk); err != syscall.EAGAIN {
		t.Errorf("FcntlFlock returned %v, expected EAGAIN", err)
	}
}

func LseekHoleSeeksToEOF(t *testing.T, mnt string) {
	fn := filepath.Join(mnt, "file.bin")
	content := bytes.Repeat([]byte("abcxyz\n"), 1024)
	if err := os.WriteFile(fn, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fd, err := syscall.Open(fn, syscall.O_RDONLY, 0644)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer syscall.Close(fd)

	off, err := unix.Seek(fd, int64(len(content)/2), unix.SEEK_HOLE)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	} else if off != int64(len(content)) {
		t.Errorf("got offset %d, want %d", off, len(content))
	}
}

func LseekEnxioCheck(t *testing.T, mnt string) {
	fn := filepath.Join(mnt, "file.bin")
	content := bytes.Repeat([]byte("abcxyz\n"), 1024)
	if err := os.WriteFile(fn, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fd, err := syscall.Open(fn, syscall.O_RDONLY, 0644)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer syscall.Close(fd)

	testCases := []struct {
		name   string
		offset int64
		whence int
	}{
		{
			name:   "Lseek SEEK_DATA where offset is at EOF returns ENXIO",
			offset: int64(len(content)),
			whence: unix.SEEK_DATA,
		},
		{
			name:   "Lseek SEEK_DATA where offset greater than EOF returns ENXIO",
			offset: int64(len(content)) + 1,
			whence: unix.SEEK_DATA,
		},
		{
			name:   "Lseek SEEK_HOLE where offset is greater than EOF returns ENXIO",
			offset: int64(len(content)) + 1,
			whence: unix.SEEK_HOLE,
		},
	}

	for _, tc := range testCases {
		_, err := unix.Seek(fd, tc.offset, tc.whence)
		if err != nil {
			if !errors.Is(err, syscall.ENXIO) {
				t.Errorf("Failed test case: %s; got %v, want %v", tc.name, err, syscall.ENXIO)
			}
		}
	}
}

// XAttr; they aren't posix but cross-platform enough to put into posixtest/
func XAttr(t *testing.T, mntDir string) {
	buf := make([]byte, 1024)
	attrNameSpace := "user"
	attrName := "xattrtest"
	attr := fmt.Sprintf("%s.%s", attrNameSpace, attrName)
	fn := mntDir + "/file"

	if err := os.WriteFile(fn, []byte{}, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := unix.Getxattr(fn, attr, buf); err == unix.ENOTSUP {
		t.Skipf("filesystem for %s does not support xattrs. Rerun this test with a $TMPDIR override", fn)
	}

	if _, err := unix.Getxattr(fn, attr, buf); err != xattr.ENOATTR {
		t.Fatalf("got %v want ENOATTR", err)
	}
	value := []byte("value")
	if err := unix.Setxattr(fn, attr, value, 0); err != nil {
		t.Fatalf("Setxattr: %v", err)
	}

	sz, err := unix.Listxattr(fn, nil)
	if err != nil {
		t.Fatalf("Listxattr: %v", err)
	}
	buf = make([]byte, sz)
	if _, err := unix.Listxattr(fn, buf); err != nil {
		t.Fatalf("Listxattr: %v", err)
	} else {
		attributes := xattr.ParseAttrNames(buf[:sz])
		found := false
		for _, a := range attributes {
			if string(a) == attr || attrNameSpace+string(a) == attr {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Listxattr: %q (not found: %q", attributes, attr)
		}
	}

	sz, err = unix.Getxattr(fn, attr, buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	if bytes.Compare(buf[:sz], value) != 0 {
		t.Fatalf("Getxattr got %q want %q", buf[:sz], value)
	}
	if err := unix.Removexattr(fn, attr); err != nil {
		t.Fatalf("Removexattr: %v", err)
	}

	if _, err := unix.Getxattr(fn, attr, buf); err != xattr.ENOATTR {
		t.Fatalf("got %v want ENOATTR", err)
	}
}

// Test if Open() is vulnerable to symlink-race attacks using two goroutines:
//
// goroutine "shuffler":
// In a loop:
// * Replace empty file "OpenSymlinkRace" with a symlink pointing to /etc/passwd
// * Replace back with empty file
//
// goroutine "opener":
// In a loop:
// * Open "OpenSymlinkRace" and call Fstat on it. Now there's three cases:
//  1. Size=0: we opened the empty file created by shuffler. Normal and uninteresting.
//  2. Size>0 but Dev number different: we (this test) opened /etc/passwd ourselves
//     because we resolved the symlink. Normal.
//  3. Size>0 and Dev number matches the FUSE mount: go-fuse opened /etc/passwd.
//     The attack has worked.
func OpenSymlinkRace(t *testing.T, mnt string) {
	path := mnt + "/OpenSymlinkRace"

	var wg sync.WaitGroup
	wg.Add(2)

	const iterations = 1000

	fd, err := syscall.Creat(path, 0600)
	if err != nil {
		t.Error(err)
		return
	}
	// Find and save the device number of the FUSE mount
	var st syscall.Stat_t
	err = syscall.Fstat(fd, &st)
	fuseMountDev := st.Dev
	if err != nil {
		t.Fatal(err)
	}
	syscall.Close(fd)

	// Shuffler
	go func() {
		defer wg.Done()
		tmp := path + ".tmp"
		for i := 0; i < iterations; i++ {
			// Stop when another thread has failed
			if t.Failed() {
				return
			}

			// Make "path" a regular file
			fd, err := syscall.Creat(tmp, 0600)
			if err != nil {
				t.Errorf("shuffler: Creat: %v", err)
				return
			}
			syscall.Close(fd)
			err = syscall.Rename(tmp, path)
			if err != nil {
				t.Error(err)
				return
			}

			// Make "path" a symlink
			err = syscall.Symlink("/etc/passwd", tmp)
			if err != nil {
				t.Errorf("shuffler: Symlink: %v", err)
				return
			}
			err = syscall.Rename(tmp, path)
			if err != nil {
				t.Errorf("shuffler: Rename: %v", err)
				return
			}
		}
	}()

	// Keep some statistics
	type statsT struct {
		EINVAL          int
		ENOENT          int
		ELOOP           int
		empty           int
		resolvedSymlink int
	}
	var stats statsT

	// Opener
	go func() {
		defer wg.Done()
		var st syscall.Stat_t

		for i := 0; i < iterations; i++ {
			// Stop when another thread has failed
			if t.Failed() {
				return
			}

			fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
			if err != nil {
				if err == syscall.EINVAL {
					// Happens when the kernel tries to read the symlink but
					// it's already a file again in the backing directory
					stats.EINVAL++
					continue
				}
				if err == syscall.ELOOP {
					// Looks like there's some symlink-safety
					stats.ELOOP++
					continue
				}
				if err == syscall.ENOENT {
					// Not sure why we get these, but we do
					stats.ENOENT++
					continue
				}
				t.Errorf("opener: Open: %v", err)
				return
			}
			err = syscall.Fstat(fd, &st)
			syscall.Close(fd)
			if err != nil {
				t.Errorf("opener: Fstat: %v", err)
				return
			}
			if st.Size == 0 {
				stats.empty++
				continue
			}
			if st.Dev != fuseMountDev {
				stats.resolvedSymlink++
			} else {
				// go-fuse has opened /etc/passwd
				t.Errorf("opener: successful symlink attack in iteration %d. We tricked go-fuse into opening /etc/passwd.", i)
				return
			}
		}
	}()

	wg.Wait()
	t.Logf("opener stats: %#v", stats)
}
