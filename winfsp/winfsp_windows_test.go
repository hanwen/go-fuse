// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

package winfsp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	gofs "github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/memfs"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
	"golang.org/x/sys/windows"
)

// These tests mount through WinFsp, and use the file system through
// the normal Windows APIs.

func mountMem(t *testing.T, mountPoint string) string {
	t.Helper()
	if !Available() {
		_, err := loadAPI()
		t.Skipf("WinFsp not available: %v", err)
	}
	one := time.Second
	opts := &gofs.Options{EntryTimeout: &one, AttrTimeout: &one}
	opts.Debug = testutil.VerboseTest()
	srv, err := Mount(mountPoint, memfs.NewRoot(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	mnt := srv.MountPoint()
	if len(mnt) == 2 && mnt[1] == ':' {
		mnt += `\`
	}
	return mnt
}

func TestMountDriveLetter(t *testing.T) {
	mnt := mountMem(t, "")
	if _, err := os.Stat(mnt); err != nil {
		t.Fatal(err)
	}
}

func TestMountDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mnt")
	mnt := mountMem(t, dir)
	if err := os.WriteFile(filepath.Join(mnt, "f"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "f")); err != nil || string(got) != "x" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestWindowsReadWrite(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "file")
	data := bytes.Repeat([]byte("0123456789abcdef"), 1<<16) // 1 MiB
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("read back %d bytes, want %d", len(got), len(data))
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(len(data)) || !fi.Mode().IsRegular() {
		t.Errorf("stat: size %d mode %v", fi.Size(), fi.Mode())
	}

	// Overwrite with something shorter.
	if err := os.WriteFile(p, []byte("short"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(p); err != nil || string(got) != "short" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestWindowsAppendTruncate(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "file")
	if err := os.WriteFile(p, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(" world")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got, _ := os.ReadFile(p); string(got) != "hello world" {
		t.Fatalf("append: got %q", got)
	}
	if err := os.Truncate(p, 4); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "hell" {
		t.Fatalf("truncate: got %q", got)
	}

	f, err = os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt([]byte("XY"), 10); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 20)
	n, err := f.ReadAt(buf, 0)
	if err != io.EOF {
		t.Fatalf("ReadAt: %v", err)
	}
	if want := "hell\x00\x00\x00\x00\x00\x00XY"; string(buf[:n]) != want {
		t.Errorf("got %q want %q", buf[:n], want)
	}
}

func TestWindowsDirectories(t *testing.T) {
	mnt := mountMem(t, "")
	d := filepath.Join(mnt, "dir", "sub")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	var want []string
	for i := range 300 {
		n := fmt.Sprintf("file%03d", i)
		want = append(want, n)
		if err := os.WriteFile(filepath.Join(d, n), []byte(n), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ents, err := os.ReadDir(d)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
		if e.IsDir() {
			t.Errorf("%s is a dir", e.Name())
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}

	if err := os.Remove(filepath.Join(mnt, "dir")); err == nil {
		t.Errorf("removing non-empty directory succeeded")
	}
	if err := os.RemoveAll(filepath.Join(mnt, "dir")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mnt, "dir")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after RemoveAll: %v", err)
	}
	if ents, err := os.ReadDir(mnt); err != nil || len(ents) != 0 {
		t.Errorf("root: %v, %v", ents, err)
	}
}

func TestWindowsRename(t *testing.T) {
	mnt := mountMem(t, "")
	a := filepath.Join(mnt, "a")
	b := filepath.Join(mnt, "b")
	if err := os.WriteFile(a, []byte("A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B"), 0644); err != nil {
		t.Fatal(err)
	}
	// os.Rename replaces the destination on Windows.
	if err := os.Rename(a, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(b); string(got) != "A" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(a); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old name: %v", err)
	}

	if err := os.Mkdir(filepath.Join(mnt, "d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(b, filepath.Join(mnt, "d", "c")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(mnt, "d"), filepath.Join(mnt, "e")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(mnt, "e", "c")); err != nil || string(got) != "A" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestWindowsCaseSensitive(t *testing.T) {
	mnt := mountMem(t, "")
	if err := os.WriteFile(filepath.Join(mnt, "abc"), []byte("lower"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mnt, "ABC"), []byte("upper"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(mnt, "abc")); string(got) != "lower" {
		t.Errorf("got %q", got)
	}
}

func TestWindowsTimesAndMode(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "f")
	if err := os.WriteFile(p, nil, 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("mtime %v, want %v", fi.ModTime(), mtime)
	}

	if err := os.Chmod(p, 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0644); err == nil {
		t.Errorf("wrote to read-only file")
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Errorf("write after chmod: %v", err)
	}
}

func TestWindowsDeleteOpen(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "f")
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// os.Open does not pass FILE_SHARE_DELETE, so deletion is a
	// sharing violation, as on NTFS.
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Errorf("remove while open: got %v, want sharing violation", err)
	}
	f.Close()

	// With FILE_SHARE_DELETE, the delete succeeds, and the file
	// goes away on the last close.
	namep, _ := windows.UTF16PtrFromString(p)
	h, err := windows.CreateFile(namep, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	f = os.NewFile(uintptr(h), p)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := f.Read(buf); err != nil || string(buf) != "data" {
		t.Errorf("read after remove: %q, %v", buf, err)
	}
	f.Close()
	if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after close: %v", err)
	}
}

func TestWindowsConcurrent(t *testing.T) {
	mnt := mountMem(t, "")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Go(func() {
			dir := filepath.Join(mnt, fmt.Sprintf("d%d", i))
			if err := os.Mkdir(dir, 0755); err != nil {
				errs <- err
				return
			}
			for j := range 20 {
				p := filepath.Join(dir, fmt.Sprintf("f%d", j))
				want := []byte(p)
				if err := os.WriteFile(p, want, 0644); err != nil {
					errs <- err
					return
				}
				if got, err := os.ReadFile(p); err != nil || !bytes.Equal(got, want) {
					errs <- fmt.Errorf("%s: %q, %v", p, got, err)
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestWindowsStatfs(t *testing.T) {
	mnt := mountMem(t, "")
	var free, total, avail uint64
	p, _ := windows.UTF16PtrFromString(mnt)
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		t.Fatal(err)
	}
	if total != 4096<<20 {
		t.Errorf("total %d", total)
	}
}

// TestWindowsClearAttributes checks FILE_ATTRIBUTE_NORMAL, which the
// WinFsp driver passes as 0, clears the read-only attribute.
func TestWindowsClearAttributes(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "f")
	if err := os.WriteFile(p, nil, 0444); err != nil {
		t.Fatal(err)
	}
	p16, _ := windows.UTF16PtrFromString(p)
	if err := windows.SetFileAttributes(p16, windows.FILE_ATTRIBUTE_NORMAL); err != nil {
		t.Fatal(err)
	}
	attrs, err := windows.GetFileAttributes(p16)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY != 0 {
		t.Errorf("attrs %#x, still read-only", attrs)
	}
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Errorf("write: %v", err)
	}
}

// TestWindowsNotifyUnmount sends notifications while unmounting, which
// must not use the file system after it is deleted.
func TestWindowsNotifyUnmount(t *testing.T) {
	if !Available() {
		t.Skip("WinFsp not available")
	}
	for range 10 {
		root := memfs.NewRoot()
		srv, err := Mount("", root, nil)
		if err != nil {
			t.Fatal(err)
		}
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				root.EmbeddedInode().NotifyEntry("f")
			}
		}()
		time.Sleep(20 * time.Millisecond)
		srv.Unmount()
		time.Sleep(20 * time.Millisecond)
		close(stop)
		wg.Wait()
	}
}

func TestStatusFromSystemError(t *testing.T) {
	for errno, want := range map[windows.Errno]ntStatus{
		windows.ERROR_FILE_NOT_FOUND:       _STATUS_OBJECT_NAME_NOT_FOUND,
		windows.ERROR_DIRECTORY:            _STATUS_NOT_A_DIRECTORY,
		windows.ERROR_ACCESS_DENIED:        _STATUS_ACCESS_DENIED,
		windows.ERROR_ALREADY_EXISTS:       _STATUS_OBJECT_NAME_COLLISION,
		windows.ERROR_DIR_NOT_EMPTY:        _STATUS_DIRECTORY_NOT_EMPTY,
		windows.ERROR_SHARING_VIOLATION:    _STATUS_SHARING_VIOLATION,
		windows.ERROR_DISK_FULL:            _STATUS_DISK_FULL,
		windows.ERROR_INVALID_PARAMETER:    _STATUS_INVALID_PARAMETER,
		windows.ERROR_NOT_SUPPORTED:        _STATUS_NOT_SUPPORTED,
		windows.ERROR_FILENAME_EXCED_RANGE: _STATUS_NAME_TOO_LONG,
	} {
		if got := statusFromErrno(errno); got != want {
			t.Errorf("%d (%v): got %v, want %v", errno, errno, got, want)
		}
	}
}

// errnoNode returns a Windows error from GETATTR, as a file system
// that wraps os calls does.
type errnoNode struct {
	gofs.Inode
}

func (n *errnoNode) Getattr(ctx context.Context, f gofs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	return gofs.ToErrno(&fs.PathError{Op: "stat", Path: "x", Err: windows.ERROR_ACCESS_DENIED})
}

type errnoRoot struct{ gofs.Inode }

func (r *errnoRoot) OnAdd(ctx context.Context) {
	r.AddChild("denied", r.NewPersistentInode(ctx, &errnoNode{}, gofs.StableAttr{}), false)
}

// TestWindowsSystemError checks that Windows errors from the file
// system reach the application.
func TestWindowsSystemError(t *testing.T) {
	if !Available() {
		t.Skip("WinFsp not available")
	}
	srv, err := Mount("", &errnoRoot{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Unmount()
	_, err = os.Stat(filepath.Join(srv.MountPoint()+`\`, "denied"))
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("got %v, want permission error", err)
	}
}

func TestWindowsSymlink(t *testing.T) {
	mnt := mountMem(t, "")
	if err := os.Mkdir(filepath.Join(mnt, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mnt, "dir", "f"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(mnt, "link")
	if err := os.Symlink(`dir\f`, link); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skip("creating symlinks needs developer mode or admin")
		}
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != `dir\f` {
		t.Errorf("readlink: %q, %v", got, err)
	}
	if got, err := os.ReadFile(link); err != nil || string(got) != "data" {
		t.Errorf("read through link: %q, %v", got, err)
	}
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("lstat: %v, %v", fi, err)
	}

	dirLink := filepath.Join(mnt, "dirlink")
	if err := os.Symlink("dir", dirLink); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dirLink, "f")); err != nil || string(got) != "data" {
		t.Errorf("read through dir link: %q, %v", got, err)
	}

	if err := os.Symlink(`C:\Windows`, filepath.Join(mnt, "abs")); err == nil {
		t.Errorf("absolute symlink succeeded")
	}
	names, err := os.ReadDir(mnt)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range names {
		if strings.HasPrefix(e.Name(), ".winfsp-symlink-") {
			t.Errorf("temporary name left: %s", e.Name())
		}
	}
}

// mountRawFS mounts raw, which should wrap fs.NewNodeFS.
func mountRawFS(t *testing.T, raw fuse.RawFileSystem) (*Server, string) {
	t.Helper()
	if !Available() {
		_, err := loadAPI()
		t.Skipf("WinFsp not available: %v", err)
	}
	srv, err := MountRaw("", raw, &fuse.MountOptions{Debug: testutil.VerboseTest()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	return srv, srv.MountPoint() + `\`
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skip("creating symlinks needs developer mode or admin")
		}
		t.Fatal(err)
	}
}

// TestWindowsDirSymlink checks that a symlink to a directory is
// reported as a directory symlink, both by name and in listings.
func TestWindowsDirSymlink(t *testing.T) {
	mnt := mountMem(t, "")
	if err := os.Mkdir(filepath.Join(mnt, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(mnt, "dirlink")
	// The target exists, so os.Symlink makes a directory symlink.
	symlinkOrSkip(t, "dir", link)

	want := uint32(windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_REPARSE_POINT)
	link16, _ := windows.UTF16PtrFromString(link)
	attrs, err := windows.GetFileAttributes(link16)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&want != want {
		t.Errorf("GetFileAttributes: %#x, want %#x set", attrs, want)
	}

	// A plain name goes to GetDirInfoByName, a wildcard to
	// ReadDirectory.
	for _, pat := range []string{link, link[:len(link)-1] + "*"} {
		pat16, _ := windows.UTF16PtrFromString(pat)
		var fd windows.Win32finddata
		h, err := windows.FindFirstFile(pat16, &fd)
		if err != nil {
			t.Fatalf("FindFirstFile(%s): %v", pat, err)
		}
		windows.FindClose(h)
		if fd.FileAttributes&want != want {
			t.Errorf("FindFirstFile(%s): %#x, want %#x set", pat, fd.FileAttributes, want)
		}
	}

	// A directory symlink is removed with RemoveDirectory.
	if err := windows.RemoveDirectory(link16); err != nil {
		t.Errorf("RemoveDirectory: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(mnt, "dir")); err != nil || !fi.IsDir() {
		t.Errorf("target after removing link: %v, %v", fi, err)
	}
}

// TestWindowsUnmountReleases checks that Unmount releases the handles
// that are still open, and tells the file system it is unmounted.
func TestWindowsUnmountReleases(t *testing.T) {
	c := &handleCounter{RawFileSystem: gofs.NewNodeFS(memfs.NewRoot(), &gofs.Options{})}
	srv, mnt := mountRawFS(t, c)
	p := filepath.Join(mnt, "f")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, err := os.Open(mnt)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Readdirnames(-1); err != nil {
		t.Fatal(err)
	}

	if err := srv.Unmount(); err != nil {
		t.Fatalf("Unmount: %v", err)
	}
	done := make(chan struct{})
	go func() {
		srv.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after Unmount")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opens != c.releases {
		t.Errorf("%d opens, %d releases", c.opens, c.releases)
	}
	if c.opendirs != c.relDirs {
		t.Errorf("%d opendirs, %d releasedirs", c.opendirs, c.relDirs)
	}
	if !c.unmounted {
		t.Errorf("OnUnmount not called")
	}
}

// TestWindowsCreateReadOnlyDir creates a directory with
// FILE_ATTRIBUTE_READONLY, which Windows ignores for directories: files
// can still be created in it.
func TestWindowsCreateReadOnlyDir(t *testing.T) {
	mnt := mountMem(t, "")
	dir := filepath.Join(mnt, "rodir")
	name, err := windows.NewNTUnicodeString(`\??\` + dir)
	if err != nil {
		t.Fatal(err)
	}
	oa := &windows.OBJECT_ATTRIBUTES{ObjectName: name}
	oa.Length = uint32(unsafe.Sizeof(*oa))
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	if err := windows.NtCreateFile(&h, windows.FILE_LIST_DIRECTORY|windows.SYNCHRONIZE, oa, &iosb, nil,
		windows.FILE_ATTRIBUTE_READONLY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0); err != nil {
		t.Fatalf("NtCreateFile: %v", err)
	}
	windows.CloseHandle(h)

	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644); err != nil {
		t.Errorf("create file in read-only directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Errorf("mkdir in read-only directory: %v", err)
	}
}

// TestWindowsMkdirOpendirFails checks that a directory whose creation
// is reported as failed does not exist afterwards.
func TestWindowsMkdirOpendirFails(t *testing.T) {
	root := memfs.NewRoot()
	_, mnt := mountRawFS(t, &failOpendir{gofs.NewNodeFS(root, &gofs.Options{})})
	if err := os.Mkdir(filepath.Join(mnt, "d"), 0755); err == nil {
		t.Fatal("Mkdir succeeded")
	}
	if ch := root.EmbeddedInode().GetChild("d"); ch != nil {
		t.Errorf("directory exists after failed Mkdir")
	}
	ents, err := os.ReadDir(mnt)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		t.Errorf("unexpected entry %q", e.Name())
	}
}

var procNtQueryDirectoryFile = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryDirectoryFile")

const fileNamesInformation = 12

// queryDirName queries dir for the single name pattern, which WinFsp
// answers with GetDirInfoByName. It uses a fresh handle, because the
// pattern is fixed by the first query on a handle.
func queryDirName(t *testing.T, dir, pattern string) (string, error) {
	t.Helper()
	dir16, _ := windows.UTF16PtrFromString(dir)
	h, err := windows.CreateFile(dir16, windows.FILE_LIST_DIRECTORY|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatalf("CreateFile(%s): %v", dir, err)
	}
	defer windows.CloseHandle(h)
	pat, err := windows.NewNTUnicodeString(pattern)
	if err != nil {
		t.Fatal(err)
	}
	var iosb windows.IO_STATUS_BLOCK
	buf := alignedBuf(4096)
	r, _, _ := procNtQueryDirectoryFile.Call(uintptr(h), 0, 0, 0, uintptr(unsafe.Pointer(&iosb)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), fileNamesInformation,
		1, uintptr(unsafe.Pointer(pat)), 1)
	if st := windows.NTStatus(r); st != windows.STATUS_SUCCESS {
		return "", st
	}
	// FILE_NAMES_INFORMATION: NextEntryOffset, FileIndex,
	// FileNameLength (bytes), FileName.
	n := *(*uint32)(unsafe.Pointer(&buf[8]))
	return string(utf16.Decode(utf16At(buf, 12, int(n/2)))), nil
}

// TestWindowsQueryDirectoryDots checks that looking up "." and ".." by
// name agrees with the listing, which contains both.
func TestWindowsQueryDirectoryDots(t *testing.T) {
	mnt := mountMem(t, "")
	dir := filepath.Join(mnt, "d")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"f", ".", ".."} {
		got, err := queryDirName(t, dir, name)
		if err != nil || got != name {
			t.Errorf("query %q: %q, %v", name, got, err)
		}
	}
	if _, err := queryDirName(t, dir, "missing"); !errors.Is(err, windows.STATUS_NO_SUCH_FILE) {
		t.Errorf("query missing: %v, want STATUS_NO_SUCH_FILE", err)
	}
}

// TestWindowsOverwriteReadOnly checks that CREATE_ALWAYS on an existing
// file applies the requested attributes.
func TestWindowsOverwriteReadOnly(t *testing.T) {
	mnt := mountMem(t, "")
	p := filepath.Join(mnt, "f")
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	p16, _ := windows.UTF16PtrFromString(p)
	h, err := windows.CreateFile(p16, windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS,
		windows.FILE_ATTRIBUTE_READONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	windows.CloseHandle(h)

	attrs, err := windows.GetFileAttributes(p16)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		t.Errorf("attrs %#x, want FILE_ATTRIBUTE_READONLY", attrs)
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() != 0 {
		t.Errorf("not truncated: %v, %v", fi, err)
	}
	// Clean up, so the read-only file does not trip RemoveAll.
	windows.SetFileAttributes(p16, windows.FILE_ATTRIBUTE_NORMAL)
}
