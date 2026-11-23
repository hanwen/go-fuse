// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/xattr"
	"golang.org/x/sys/unix"
)

// testReadOnly tries every operation that modifies the tree under
// mnt, both by path and through read-only file descriptors, and
// checks that each fails and that the tree is unchanged afterwards.
// The tree must contain at least one regular file and one
// subdirectory.
func testReadOnly(t *testing.T, mnt string) {
	before := readOnlySnapshot(t, mnt)

	var names, dirs []string
	var haveFile bool
	for rel, e := range before {
		switch e.mode & syscall.S_IFMT {
		case syscall.S_IFREG:
			haveFile = true
		case syscall.S_IFDIR:
			dirs = append(dirs, filepath.Join(mnt, rel))
		}
		if rel != "." {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	sort.Strings(dirs)
	if !haveFile || len(dirs) < 2 {
		t.Fatalf("%s must contain a regular file and a directory", mnt)
	}

	mustFail := func(op string, p string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s %s: succeeded", op, p)
		}
	}
	mustFailOpen := func(p string, flags int) {
		t.Helper()
		fd, err := unix.Open(p, flags, 0644)
		if err == nil {
			unix.Close(fd)
		}
		mustFail(fmt.Sprintf("open(%#x)", flags), p, err)
	}

	uid, gid := os.Getuid(), os.Getgid()
	ts := []unix.Timespec{unix.NsecToTimespec(1e9), unix.NsecToTimespec(1e9)}
	now := []unix.Timespec{{Nsec: unix.UTIME_NOW}, {Nsec: unix.UTIME_NOW}}
	tvs := []unix.Timeval{unix.NsecToTimeval(1e9), unix.NsecToTimeval(1e9)}
	const newName = "readonly-new"
	const attr = "user.readonly"

	// Operations through a file descriptor opened for reading only;
	// the kernel sends these with a file handle.
	fdOps := func(rel string, flags int) {
		t.Helper()
		p := filepath.Join(mnt, rel)
		fd, err := unix.Open(p, flags, 0)
		if err != nil {
			t.Logf("open %s: %v; skipping fd operations", p, err)
			return
		}
		defer unix.Close(fd)
		mustFail("fchmod", p, unix.Fchmod(fd, 0700))
		mustFail("fchown", p, unix.Fchown(fd, uid, gid))
		mustFail("futimes", p, unix.Futimes(fd, tvs))
		mustFail("fsetxattr", p, unix.Fsetxattr(fd, attr, []byte("x"), 0))
		mustFail("fremovexattr", p, unix.Fremovexattr(fd, attr))
		for _, a := range before[rel].xattrNames() {
			mustFail("fremovexattr "+a, p, unix.Fremovexattr(fd, a))
		}
		if flags&unix.O_DIRECTORY != 0 {
			mustFail("mkdirat", p, unix.Mkdirat(fd, newName, 0755))
			mustFail("symlinkat", p, unix.Symlinkat("target", fd, newName))
			mustFail("mknodat", p, unix.Mknodat(fd, newName, unix.S_IFIFO|0644, 0))
			fd2, err := unix.Openat(fd, newName, unix.O_CREAT|unix.O_WRONLY, 0644)
			if err == nil {
				unix.Close(fd2)
			}
			mustFail("openat(O_CREAT)", p, err)
			return
		}
		mustFail("ftruncate", p, unix.Ftruncate(fd, 0))
		mustFail("fallocate", p, unix.Fallocate(fd, 0, 0, 4096))
		_, err = unix.Write(fd, []byte("x"))
		mustFail("write", p, err)
		_, err = unix.CopyFileRange(fd, nil, fd, nil, 1, 0)
		mustFail("copy_file_range", p, err)
		if data, err := unix.Mmap(fd, 0, 1, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED); err == nil {
			unix.Munmap(data)
			t.Errorf("mmap(PROT_WRITE, MAP_SHARED) %s: succeeded", p)
		}
		mustFail("linkat(AT_EMPTY_PATH)", p, unix.Linkat(fd, "", unix.AT_FDCWD, filepath.Join(mnt, newName), unix.AT_EMPTY_PATH))
	}

	for _, rel := range append([]string{"."}, names...) {
		e := before[rel]
		p := filepath.Join(mnt, rel)

		mustFail("utimensat", p, unix.UtimesNanoAt(unix.AT_FDCWD, p, ts, unix.AT_SYMLINK_NOFOLLOW))
		mustFail("utimensat(UTIME_NOW)", p, unix.UtimesNanoAt(unix.AT_FDCWD, p, now, unix.AT_SYMLINK_NOFOLLOW))
		mustFail("lchown", p, unix.Lchown(p, uid, gid))
		mustFail("lchown(other)", p, unix.Lchown(p, uid+1, gid+1))
		for _, flags := range []int{0, unix.XATTR_CREATE, unix.XATTR_REPLACE} {
			mustFail(fmt.Sprintf("lsetxattr(%d)", flags), p, unix.Lsetxattr(p, attr, []byte("x"), flags))
		}
		mustFail("lremovexattr", p, unix.Lremovexattr(p, attr))
		for _, a := range e.xattrNames() {
			mustFail("lsetxattr(XATTR_REPLACE) "+a, p, unix.Lsetxattr(p, a, []byte("x"), unix.XATTR_REPLACE))
			mustFail("lremovexattr "+a, p, unix.Lremovexattr(p, a))
		}

		if rel != "." {
			for _, d := range dirs {
				dst := filepath.Join(d, newName)
				mustFail("rename to "+dst, p, unix.Rename(p, dst))
				mustFail("rename(NOREPLACE) to "+dst, p, unix.Renameat2(unix.AT_FDCWD, p, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE))
				mustFail("rename(WHITEOUT) to "+dst, p, unix.Renameat2(unix.AT_FDCWD, p, unix.AT_FDCWD, dst, unix.RENAME_WHITEOUT))
				if e.mode&syscall.S_IFMT != syscall.S_IFDIR {
					mustFail("link to "+dst, p, unix.Link(p, dst))
				}
			}
			for _, other := range names {
				if other == rel {
					continue
				}
				o := filepath.Join(mnt, other)
				mustFail("rename(EXCHANGE) with "+o, p, unix.Renameat2(unix.AT_FDCWD, p, unix.AT_FDCWD, o, unix.RENAME_EXCHANGE))
				mustFail("rename onto "+o, p, unix.Rename(p, o))
			}
		}

		switch e.mode & syscall.S_IFMT {
		case syscall.S_IFDIR:
			c := filepath.Join(p, newName)
			mustFail("mkdir", c, unix.Mkdir(c, 0755))
			mustFailOpen(c, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY)
			mustFailOpen(c, unix.O_CREAT|unix.O_WRONLY)
			mustFailOpen(c, unix.O_CREAT|unix.O_RDONLY)
			mustFailOpen(c, unix.O_CREAT|unix.O_RDWR|unix.O_TRUNC)
			mustFailOpen(p, unix.O_TMPFILE|unix.O_WRONLY)
			mustFailOpen(p, unix.O_TMPFILE|unix.O_RDWR|unix.O_EXCL)
			for _, mode := range []uint32{unix.S_IFREG, unix.S_IFIFO, unix.S_IFSOCK, unix.S_IFCHR} {
				mustFail(fmt.Sprintf("mknod(%o)", mode), c, unix.Mknod(c, mode|0644, 0))
			}
			mustFail("symlink", c, unix.Symlink("target", c))
			mustFail("chmod", p, unix.Chmod(p, 0700))
			mustFail("chmod(sticky)", p, unix.Chmod(p, (e.mode&07777)|unix.S_ISVTX))
			if rel != "." {
				mustFail("rmdir", p, unix.Rmdir(p))
			}
			fdOps(rel, unix.O_RDONLY|unix.O_DIRECTORY)
		case syscall.S_IFREG:
			mustFailOpen(p, unix.O_WRONLY)
			mustFailOpen(p, unix.O_RDWR)
			mustFailOpen(p, unix.O_WRONLY|unix.O_APPEND)
			mustFailOpen(p, unix.O_RDONLY|unix.O_TRUNC)
			mustFailOpen(p, unix.O_CREAT|unix.O_WRONLY)
			mustFailOpen(p, unix.O_CREAT|unix.O_RDONLY|unix.O_TRUNC)
			mustFail("truncate(0)", p, unix.Truncate(p, 0))
			mustFail("truncate(grow)", p, unix.Truncate(p, e.size+4096))
			mustFail("chmod", p, unix.Chmod(p, 0600))
			mustFail("chmod(setuid)", p, unix.Chmod(p, (e.mode&07777)|unix.S_ISUID|unix.S_ISGID))
			mustFail("unlink", p, unix.Unlink(p))
			fdOps(rel, unix.O_RDONLY)
		default:
			mustFail("unlink", p, unix.Unlink(p))
		}
	}

	after := readOnlySnapshot(t, mnt)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("tree changed:\nbefore %v\nafter  %v", before, after)
	}
}

type readOnlyEntry struct {
	mode     uint32
	size     int64
	nlink    uint64
	uid, gid uint32
	mtime    int64
	content  string
	xattrs   map[string]string
}

func (e readOnlyEntry) xattrNames() []string {
	var r []string
	for k := range e.xattrs {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

func readOnlySnapshot(t *testing.T, mnt string) map[string]readOnlyEntry {
	t.Helper()
	result := map[string]readOnlyEntry{}
	err := filepath.Walk(mnt, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(mnt, p)
		if err != nil {
			return err
		}
		st := fi.Sys().(*syscall.Stat_t)
		e := readOnlyEntry{
			mode:   st.Mode,
			size:   fi.Size(),
			nlink:  uint64(st.Nlink),
			uid:    st.Uid,
			gid:    st.Gid,
			mtime:  fi.ModTime().UnixNano(),
			xattrs: map[string]string{},
		}
		switch fi.Mode().Type() {
		case 0:
			if data, err := os.ReadFile(p); err != nil {
				e.content = "error: " + err.Error()
			} else {
				e.content = string(data)
			}
		case os.ModeSymlink:
			if l, err := os.Readlink(p); err != nil {
				e.content = "error: " + err.Error()
			} else {
				e.content = l
			}
		}

		buf := make([]byte, 64<<10)
		if n, err := unix.Llistxattr(p, buf); err == nil {
			for _, a := range xattr.ParseAttrNames(buf[:n]) {
				val := make([]byte, 64<<10)
				if m, err := unix.Lgetxattr(p, string(a), val); err != nil {
					e.xattrs[string(a)] = "error: " + err.Error()
				} else {
					e.xattrs[string(a)] = string(val[:m])
				}
			}
		}
		result[rel] = e
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return result
}

// readOnlyFile is a file that can only be opened for reading.
type readOnlyFile struct {
	Inode
	data []byte
}

var _ = (NodeOpener)((*readOnlyFile)(nil))
var _ = (NodeReader)((*readOnlyFile)(nil))
var _ = (NodeGetattrer)((*readOnlyFile)(nil))

func (f *readOnlyFile) Open(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_ACCMODE|syscall.O_TRUNC) != syscall.O_RDONLY {
		return nil, 0, syscall.EROFS
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (f *readOnlyFile) Read(ctx context.Context, fh FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := min(off+int64(len(dest)), int64(len(f.data)))
	off = min(off, end)
	return fuse.ReadResultData(f.data[off:end]), 0
}

func (f *readOnlyFile) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0444
	out.Size = uint64(len(f.data))
	return 0
}

func TestStaticTreeReadOnly(t *testing.T) {
	root := &Inode{}
	mnt, _ := testMount(t, root, &Options{
		OnAdd: func(ctx context.Context) {
			dir := root.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFDIR})
			root.AddChild("dir", dir, false)
			for _, p := range []*Inode{root, dir} {
				p.AddChild("file", p.NewPersistentInode(ctx, &readOnlyFile{data: []byte("hello")}, StableAttr{Mode: syscall.S_IFREG}), false)
				p.AddChild("empty", p.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFREG}), false)
			}
		},
	})
	testReadOnly(t, mnt)
}
