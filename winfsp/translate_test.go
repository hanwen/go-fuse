// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || windows

package winfsp

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/memfs"
	"github.com/hanwen/go-fuse/v3/internal/testutil"
)

// These tests call the WinFsp callbacks directly, playing the role of
// the WinFsp DLL.

func newTestFS(t *testing.T, root fs.InodeEmbedder, opts *fs.Options) *fileSystem {
	t.Helper()
	if root == nil {
		root = memfs.NewRoot()
	}
	if opts == nil {
		opts = &fs.Options{}
	}
	opts.Debug = testutil.VerboseTest()
	raw := fs.NewNodeFS(root, opts)
	mo := opts.MountOptions
	fsys, err := newFileSystem(raw, &mo, fuse.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}

func (fsys *fileSystem) createFile(t *testing.T, path string, data []byte) {
	t.Helper()
	f, _, st := fsys.create(path, 0, 0, 0644)
	mustOK(t, "create "+path, st)
	if len(data) > 0 {
		n, fi, st := fsys.write(f, data, 0, false, false)
		mustOK(t, "write", st)
		if n != len(data) || fi.FileSize != uint64(len(data)) {
			t.Fatalf("write: n=%d size=%d, want %d", n, fi.FileSize, len(data))
		}
	}
	fsys.cleanup(f, path, 0)
	fsys.close(f)
}

func (fsys *fileSystem) remove(t *testing.T, path string) ntStatus {
	t.Helper()
	f, _, st := fsys.open(path, 0)
	if st != _STATUS_SUCCESS {
		return st
	}
	defer fsys.close(f)
	if st := fsys.canDelete(f); st != _STATUS_SUCCESS {
		fsys.cleanup(f, path, 0)
		return st
	}
	fsys.cleanup(f, path, cleanupDelete)
	return _STATUS_SUCCESS
}

func (fsys *fileSystem) list(t *testing.T, path string, bufSize int) []string {
	t.Helper()
	f, _, st := fsys.open(path, _FILE_READ_DATA)
	mustOK(t, "open "+path, st)
	defer fsys.close(f)

	var names []string
	var marker *string
	buf := make([]byte, bufSize)
	for i := 0; ; i++ {
		if i > 1000 {
			t.Fatal("listing does not end")
		}
		n, st := fsys.readDirectory(f, marker, buf)
		mustOK(t, "readDirectory", st)
		got, _, end := parseDirInfos(buf[:n])
		names = append(names, got...)
		if end {
			break
		}
		if len(got) == 0 {
			t.Fatalf("buffer of %d bytes holds no entry", bufSize)
		}
		last := got[len(got)-1]
		marker = &last
	}
	return names
}

func TestReadWrite(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	data := bytes.Repeat([]byte("abcdefgh"), 100000)
	fsys.createFile(t, `\file`, data)

	attrs, a, st := fsys.getSecurityByName(`\file`)
	mustOK(t, "getSecurityByName", st)
	if attrs&_FILE_ATTRIBUTE_DIRECTORY != 0 || a.Size != uint64(len(data)) {
		t.Errorf("got attrs %x size %d", attrs, a.Size)
	}

	f, fi, st := fsys.open(`\file`, _FILE_READ_DATA)
	mustOK(t, "open", st)
	defer fsys.close(f)
	if fi.FileSize != uint64(len(data)) {
		t.Errorf("size %d, want %d", fi.FileSize, len(data))
	}
	got := make([]byte, len(data)+10)
	n, st := fsys.read(f, got, 0)
	mustOK(t, "read", st)
	if !bytes.Equal(got[:n], data) {
		t.Errorf("read back %d bytes, mismatch", n)
	}
	if _, st := fsys.read(f, got, uint64(len(data))); st != _STATUS_END_OF_FILE {
		t.Errorf("read at EOF: %v", st)
	}
}

func TestAppendConstrained(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	fsys.createFile(t, `\f`, []byte("hello"))
	f, _, st := fsys.open(`\f`, _FILE_APPEND_DATA)
	mustOK(t, "open", st)
	defer fsys.close(f)
	if _, fi, st := fsys.write(f, []byte(" world"), 0, true, false); st != _STATUS_SUCCESS || fi.FileSize != 11 {
		t.Fatalf("append: %v size %d", st, fi.FileSize)
	}
	// Paging writes must not extend the file.
	n, fi, st := fsys.write(f, []byte("0123456789ABCDEF"), 8, false, true)
	mustOK(t, "constrained write", st)
	if n != 3 || fi.FileSize != 11 {
		t.Fatalf("constrained: n=%d size=%d", n, fi.FileSize)
	}
	fi, st = fsys.setFileSize(f, 4, false)
	mustOK(t, "setFileSize", st)
	if fi.FileSize != 4 {
		t.Errorf("size %d", fi.FileSize)
	}
	fi, st = fsys.overwrite(f, 0, false)
	mustOK(t, "overwrite", st)
	if fi.FileSize != 0 {
		t.Errorf("overwrite: size %d", fi.FileSize)
	}
}

func TestNotFound(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	fsys.createFile(t, `\file`, nil)
	for p, want := range map[string]ntStatus{
		`\missing`:        _STATUS_OBJECT_NAME_NOT_FOUND,
		`\missing\x`:      _STATUS_OBJECT_PATH_NOT_FOUND,
		`\file\x`:         _STATUS_OBJECT_PATH_NOT_FOUND,
		`\missing\x\y\z`:  _STATUS_OBJECT_PATH_NOT_FOUND,
		`\file\x\y\z\..`:  _STATUS_OBJECT_PATH_NOT_FOUND,
		`\file\x\y\z\..x`: _STATUS_OBJECT_PATH_NOT_FOUND,
	} {
		if _, _, st := fsys.getSecurityByName(p); st != want {
			t.Errorf("%s: got %v want %v", p, st, want)
		}
	}
	if _, _, st := fsys.create(`\missing\x`, 0, 0, 0644); st != _STATUS_OBJECT_PATH_NOT_FOUND {
		t.Errorf("create in missing dir: %v", st)
	}
	if _, _, st := fsys.create(`\file`, 0, 0, 0644); st != _STATUS_OBJECT_NAME_COLLISION {
		t.Errorf("create existing: %v", st)
	}
}

func TestDirectory(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, fi, st := fsys.create(`\dir`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	if fi.FileAttributes&_FILE_ATTRIBUTE_DIRECTORY == 0 {
		t.Errorf("attrs %x", fi.FileAttributes)
	}
	fsys.close(d)

	var want []string
	for i := range 50 {
		name := fmt.Sprintf("file%03d-with-a-long-name", i)
		fsys.createFile(t, `\dir\`+name, []byte(name))
		want = append(want, name)
	}
	if got := fsys.list(t, `\`, 4096); !slices.Equal(got, []string{"dir"}) {
		t.Errorf("root: got %v", got)
	}
	want = append([]string{".", ".."}, want...)
	for _, sz := range []int{200, 1000, 65536} {
		if got := fsys.list(t, `\dir`, sz); !slices.Equal(got, want) {
			t.Errorf("buf %d: got %v, want %v", sz, got, want)
		}
	}

	if st := fsys.remove(t, `\dir`); st != _STATUS_DIRECTORY_NOT_EMPTY {
		t.Errorf("remove non-empty: %v", st)
	}
	for _, n := range want[2:] {
		mustOK(t, "remove", fsys.remove(t, `\dir\`+n))
	}
	mustOK(t, "rmdir", fsys.remove(t, `\dir`))
	if _, _, st := fsys.getSecurityByName(`\dir`); st != _STATUS_OBJECT_NAME_NOT_FOUND {
		t.Errorf("after rmdir: %v", st)
	}
}

// TestListDotsFirst checks that "." and ".." come first, also before
// names that sort before '.', and that markers resume after them.
func TestListDotsFirst(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, _, st := fsys.create(`\dir`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	fsys.close(d)
	for _, n := range []string{"b", "-a", "!c"} {
		fsys.createFile(t, `\dir\`+n, nil)
	}
	want := []string{".", "..", "!c", "-a", "b"}
	for _, sz := range []int{dirInfoSize + 8, 4096} {
		if got := fsys.list(t, `\dir`, sz); !slices.Equal(got, want) {
			t.Errorf("buf %d: got %q, want %q", sz, got, want)
		}
	}
}

func TestRename(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, _, st := fsys.create(`\dir`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	fsys.close(d)
	fsys.createFile(t, `\a`, []byte("a"))
	fsys.createFile(t, `\b`, []byte("bb"))

	f, _, st := fsys.open(`\a`, 0)
	mustOK(t, "open", st)
	if st := fsys.rename(`\a`, `\b`, false); st != _STATUS_OBJECT_NAME_COLLISION {
		t.Errorf("rename noreplace: %v", st)
	}
	if st := fsys.rename(`\a`, `\dir`, true); st != _STATUS_ACCESS_DENIED {
		t.Errorf("rename onto dir: %v", st)
	}
	mustOK(t, "rename", fsys.rename(`\a`, `\dir\c`, false))
	fsys.close(f)

	f, _, st = fsys.open(`\dir\c`, 0)
	mustOK(t, "open", st)
	mustOK(t, "rename replace", fsys.rename(`\dir\c`, `\b`, true))
	fsys.close(f)

	_, a, st := fsys.getSecurityByName(`\b`)
	mustOK(t, "stat", st)
	if a.Size != 1 {
		t.Errorf("size %d, want 1", a.Size)
	}
	if _, _, st := fsys.getSecurityByName(`\a`); st != _STATUS_OBJECT_NAME_NOT_FOUND {
		t.Errorf("old name: %v", st)
	}
	if got := fsys.list(t, `\`, 4096); !slices.Equal(got, []string{"b", "dir"}) {
		t.Errorf("got %v", got)
	}
}

func TestSetBasicInfo(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	fsys.createFile(t, `\f`, nil)
	f, _, st := fsys.open(`\f`, 0)
	mustOK(t, "open", st)
	defer fsys.close(f)

	mtime := time.Date(2001, 2, 3, 4, 5, 6, 700, time.UTC)
	ft := filetime(uint64(mtime.Unix()), uint32(mtime.Nanosecond()))
	fi, st := fsys.setBasicInfo(f, _FILE_ATTRIBUTE_READONLY, 0, 0, ft, 0)
	mustOK(t, "setBasicInfo", st)
	if got := unixTime(fi.LastWriteTime); !got.Equal(mtime.Truncate(100)) {
		t.Errorf("mtime %v, want %v", got, mtime)
	}
	if fi.FileAttributes&_FILE_ATTRIBUTE_READONLY == 0 {
		t.Errorf("attrs %x", fi.FileAttributes)
	}
	fi, st = fsys.setBasicInfo(f, _INVALID_FILE_ATTRIBUTES, 0, 0, 0, 0)
	mustOK(t, "setBasicInfo", st)
	if fi.FileAttributes&_FILE_ATTRIBUTE_READONLY == 0 {
		t.Errorf("INVALID_FILE_ATTRIBUTES changed attrs: %x", fi.FileAttributes)
	}
	// The WinFsp driver sends 0 for FILE_ATTRIBUTE_NORMAL.
	fi, st = fsys.setBasicInfo(f, 0, 0, 0, 0, 0)
	mustOK(t, "setBasicInfo", st)
	if fi.FileAttributes&_FILE_ATTRIBUTE_READONLY != 0 {
		t.Errorf("attrs %x", fi.FileAttributes)
	}
}

func TestOpenFlags(t *testing.T) {
	for access, want := range map[uint32]uint32{
		0:                                   syscall.O_RDONLY,
		_FILE_READ_DATA:                     syscall.O_RDONLY,
		_FILE_EXECUTE:                       syscall.O_RDONLY,
		_FILE_WRITE_DATA:                    syscall.O_WRONLY,
		_FILE_APPEND_DATA:                   syscall.O_WRONLY,
		_FILE_READ_DATA | _FILE_WRITE_DATA:  syscall.O_RDWR,
		_FILE_READ_DATA | _FILE_APPEND_DATA: syscall.O_RDWR,
		_FILE_EXECUTE | _FILE_WRITE_DATA:    syscall.O_RDWR,
	} {
		if got := openFlags(access); got != want {
			t.Errorf("openFlags(%#x) = %#o, want %#o", access, got, want)
		}
	}
}

func TestVolumeInfo(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	vi, st := fsys.getVolumeInfo()
	mustOK(t, "getVolumeInfo", st)
	if vi.TotalSize != 4096<<20 {
		t.Errorf("got %+v", vi)
	}
}

func TestSymlink(t *testing.T) {
	root := memfs.NewRoot()
	fsys := newTestFS(t, root, nil)
	n, _, errno := fsys.tree.Symlink(&fsys.caller, fsys.tree.Root(), "link", "target")
	if errno != 0 {
		t.Fatal(errno)
	}
	fsys.tree.Unref(n)

	attrs, _, nst := fsys.getSecurityByName(`\link`)
	mustOK(t, "stat", nst)
	if attrs&_FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		t.Errorf("attrs %x", attrs)
	}
	target, nst := fsys.readlink(`\link`)
	mustOK(t, "readlink", nst)
	if target != "target" {
		t.Errorf("got %q", target)
	}
}

// TestNotify checks that entry invalidations from the file system
// reach the translator and WinFsp.
func TestNotify(t *testing.T) {
	root := memfs.NewRoot()
	opts := &fs.Options{}
	sec := time.Hour
	opts.EntryTimeout = &sec
	fsys := newTestFS(t, root, opts)
	fsys.createFile(t, `\f`, nil)

	type note struct {
		path string
		c    change
	}
	got := make(chan note, 1)
	fsys.setNotify(func(p string, c change) { got <- note{p, c} })
	expect := func(want note) {
		t.Helper()
		select {
		case n := <-got:
			if n != want {
				t.Errorf("got %v, want %v", n, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no notification")
		}
	}

	if errno := root.EmbeddedInode().NotifyEntry("f"); errno != 0 {
		t.Fatalf("NotifyEntry: %v", errno)
	}
	expect(note{`\f`, changeEntry})
	if l := fsys.tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}

	f, _, st := fsys.open(`\f`, 0)
	mustOK(t, "open", st)
	defer fsys.close(f)
	child := root.EmbeddedInode().GetChild("f")
	if errno := child.NotifyContent(0, 0); errno != 0 {
		t.Fatalf("NotifyContent: %v", errno)
	}
	expect(note{`\f`, changeInode})
	if errno := root.EmbeddedInode().NotifyDelete("f", child); errno != 0 {
		t.Fatalf("NotifyDelete: %v", errno)
	}
	expect(note{`\f`, changeRemoved})
}

// notifyOnConnect starts sending notifications as soon as it is
// connected, which is before INIT has returned and the translator has
// its tree.
type notifyOnConnect struct {
	fuse.RawFileSystem
	done chan struct{}
}

func (r *notifyOnConnect) OnConnect(c *fuse.Conn) {
	r.RawFileSystem.OnConnect(c)
	go func() {
		defer close(r.done)
		n := c.Notify()
		n.InvalInode(fuse.FUSE_ROOT_ID, 0, 0)
		n.InvalEntry(fuse.FUSE_ROOT_ID, "x")
		n.Delete(fuse.FUSE_ROOT_ID, 0, "x")
	}()
}

// TestNotifyDuringInit is mostly useful with -race.
func TestNotifyDuringInit(t *testing.T) {
	raw := &notifyOnConnect{
		RawFileSystem: fs.NewNodeFS(memfs.NewRoot(), &fs.Options{}),
		done:          make(chan struct{}),
	}
	fsys, err := newFileSystem(raw, &fuse.MountOptions{}, fuse.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	<-raw.done
	fsys.createFile(t, `\f`, nil)
}

// TestNotifyOrder checks that notifications reach WinFsp in the order
// the file system sent them.
func TestNotifyOrder(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	const n = 200
	got := make(chan string, n)
	fsys.setNotify(func(p string, c change) { got <- p })
	var want []string
	for i := range n {
		p := fmt.Sprintf("/f%d", i)
		want = append(want, strings.ReplaceAll(p, "/", `\`))
		fsys.sendNotify(p, changeInode)
	}
	for i := range n {
		select {
		case p := <-got:
			if p != want[i] {
				t.Fatalf("notification %d: got %q, want %q", i, p, want[i])
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("notification %d missing", i)
		}
	}
}

// TestNotifyAfterStop checks that queued notifications are dropped once
// notifications are turned off, as on unmount.
func TestNotifyAfterStop(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	block := make(chan struct{})
	var mu sync.Mutex
	var got []string
	fsys.setNotify(func(p string, c change) {
		<-block
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	for i := range 10 {
		fsys.sendNotify(fmt.Sprintf("/f%d", i), changeInode)
	}
	fsys.setNotify(nil)
	close(block)
	fsys.sendNotify("/late", changeInode)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	// The notification in flight when notify was turned off may
	// still arrive.
	if len(got) > 1 {
		t.Errorf("got %v after stop", got)
	}
}

// rewindDir is a directory whose listing can only be restarted, not
// seeked, as with file systems that stream their listing.
type rewindDir struct {
	fs.Inode
	n int
}

var _ = (fs.NodeReaddirer)((*rewindDir)(nil))
var _ = (fs.NodeOnAdder)((*rewindDir)(nil))

func (d *rewindDir) OnAdd(ctx context.Context) {
	for i := range d.n {
		ch := d.NewPersistentInode(ctx, &fs.MemRegularFile{}, fs.StableAttr{})
		d.AddChild(fmt.Sprintf("f%04d", i), ch, false)
	}
}

func (d *rewindDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	var es []fuse.DirEntry
	for name, ch := range d.Children() {
		es = append(es, fuse.DirEntry{Name: name, Ino: ch.StableAttr().Ino, Mode: ch.Mode()})
	}
	// Without Seekdir, the stream can only be restarted.
	return struct{ fs.DirStream }{fs.NewListDirStream(es)}, 0
}

// rewindDirSize is large enough to need several READDIRPLUS requests.
const rewindDirSize = 2000

type rewindRoot struct{ fs.Inode }

func (r *rewindRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(ctx, &rewindDir{n: rewindDirSize}, fs.StableAttr{Mode: fuse.S_IFDIR})
	r.AddChild("dir", ch, false)
}

// TestCanDeleteDuringList checks that CanDelete on a directory handle
// does not disturb a listing on the same handle.
func TestCanDeleteDuringList(t *testing.T) {
	fsys := newTestFS(t, &rewindRoot{}, nil)
	f, _, st := fsys.open(`\dir`, _FILE_READ_DATA)
	mustOK(t, "open", st)
	defer fsys.close(f)

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
			if st := fsys.canDelete(f); st != _STATUS_DIRECTORY_NOT_EMPTY {
				t.Errorf("canDelete: %v", st)
				return
			}
		}
	}()
	buf := make([]byte, 1<<20)
	for i := range 50 {
		sz, st := fsys.readDirectory(f, nil, buf)
		if st != _STATUS_SUCCESS {
			t.Errorf("listing %d: %v", i, st)
			break
		}
		names, _, _ := parseDirInfos(buf[:sz])
		if len(names) != rewindDirSize+2 {
			t.Errorf("listing %d: got %d entries, want %d", i, len(names), rewindDirSize+2)
			break
		}
	}
	close(stop)
	wg.Wait()
}

// TestSetSymlink follows how Windows creates a symlink: create a file
// or directory, then set a reparse point on it.
func TestSetSymlink(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	for _, dir := range []bool{false, true} {
		path := `\file`
		var opts uint32
		if dir {
			path = `\dir`
			opts = _FILE_DIRECTORY_FILE
		}
		f, _, st := fsys.create(path, opts, 0, 0755)
		mustOK(t, "create", st)
		mustOK(t, "setSymlink", fsys.setSymlink(f, path, "target"))
		fsys.cleanup(f, path, 0)
		fsys.close(f)

		attrs, _, st := fsys.getSecurityByName(path)
		mustOK(t, "stat", st)
		if attrs&_FILE_ATTRIBUTE_REPARSE_POINT == 0 {
			t.Errorf("%s: attrs %x", path, attrs)
		}
		target, st := fsys.readlink(path)
		mustOK(t, "readlink", st)
		if target != "target" {
			t.Errorf("%s: target %q", path, target)
		}
	}

	d, _, st := fsys.create(`\full`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	fsys.createFile(t, `\full\x`, nil)
	if st := fsys.setSymlink(d, `\full`, "target"); st != _STATUS_DIRECTORY_NOT_EMPTY {
		t.Errorf("non-empty directory: %v", st)
	}
	fsys.close(d)

	if got, want := fsys.list(t, `\`, 4096), []string{"dir", "file", "full"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v (temporary name left?)", got, want)
	}
	fsys.tree.ForgetAll()
	if l := fsys.tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}
}

// handleCounter counts the file handles that the file system hands
// out and gets back.
type handleCounter struct {
	fuse.RawFileSystem

	mu                sync.Mutex
	opens, releases   int
	opendirs, relDirs int
	unmounted         bool
}

func (c *handleCounter) Open(ctx context.Context, in *fuse.OpenIn, out *fuse.OpenOut) syscall.Errno {
	errno := c.RawFileSystem.Open(ctx, in, out)
	if errno == 0 {
		c.mu.Lock()
		c.opens++
		c.mu.Unlock()
	}
	return errno
}

func (c *handleCounter) Create(ctx context.Context, in *fuse.CreateIn, name string, out *fuse.CreateOut) syscall.Errno {
	errno := c.RawFileSystem.Create(ctx, in, name, out)
	if errno == 0 {
		c.mu.Lock()
		c.opens++
		c.mu.Unlock()
	}
	return errno
}

func (c *handleCounter) Release(ctx context.Context, in *fuse.ReleaseIn) {
	c.mu.Lock()
	c.releases++
	c.mu.Unlock()
	c.RawFileSystem.Release(ctx, in)
}

func (c *handleCounter) OpenDir(ctx context.Context, in *fuse.OpenIn, out *fuse.OpenOut) syscall.Errno {
	errno := c.RawFileSystem.OpenDir(ctx, in, out)
	if errno == 0 {
		c.mu.Lock()
		c.opendirs++
		c.mu.Unlock()
	}
	return errno
}

func (c *handleCounter) ReleaseDir(ctx context.Context, in *fuse.ReleaseIn) {
	c.mu.Lock()
	c.relDirs++
	c.mu.Unlock()
	c.RawFileSystem.ReleaseDir(ctx, in)
}

func (c *handleCounter) OnUnmount() {
	c.mu.Lock()
	c.unmounted = true
	c.mu.Unlock()
	c.RawFileSystem.OnUnmount()
}

// failOpendir fails OPENDIR on everything but the root.
type failOpendir struct {
	fuse.RawFileSystem
}

func (f *failOpendir) OpenDir(ctx context.Context, in *fuse.OpenIn, out *fuse.OpenOut) syscall.Errno {
	if in.NodeId != fuse.FUSE_ROOT_ID {
		return syscall.EIO
	}
	return f.RawFileSystem.OpenDir(ctx, in, out)
}

func newTestRawFS(t *testing.T, raw fuse.RawFileSystem) *fileSystem {
	t.Helper()
	fsys, err := newFileSystem(raw, &fuse.MountOptions{Debug: testutil.VerboseTest()}, fuse.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}

// TestDirSymlinkAttributes checks that a symlink to a directory has
// FILE_ATTRIBUTE_DIRECTORY, so Windows treats it as a directory
// symlink.
func TestDirSymlinkAttributes(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, _, st := fsys.create(`\dir`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	fsys.close(d)
	for _, l := range []struct{ name, target string }{{"dirlink", "dir"}, {"filelink", "missing"}} {
		f, _, st := fsys.create(`\`+l.name, 0, 0, 0644)
		mustOK(t, "create", st)
		mustOK(t, "setSymlink", fsys.setSymlink(f, `\`+l.name, l.target))
		fsys.close(f)
	}
	want := map[string]uint32{
		"dir":      _FILE_ATTRIBUTE_DIRECTORY,
		"dirlink":  _FILE_ATTRIBUTE_DIRECTORY | _FILE_ATTRIBUTE_REPARSE_POINT,
		"filelink": _FILE_ATTRIBUTE_REPARSE_POINT,
	}
	const mask = _FILE_ATTRIBUTE_DIRECTORY | _FILE_ATTRIBUTE_REPARSE_POINT

	root, _, st := fsys.open(`\`, _FILE_READ_DATA)
	mustOK(t, "open root", st)
	defer fsys.close(root)
	buf := make([]byte, 4096)
	n, st := fsys.readDirectory(root, nil, buf)
	mustOK(t, "readDirectory", st)
	names, infos, _ := parseDirInfos(buf[:n])
	for i, name := range names {
		if got := infos[i].FileAttributes & mask; got != want[name] {
			t.Errorf("readDirectory %s: attrs %#x, want %#x", name, got, want[name])
		}
	}
	for name, w := range want {
		attrs, _, st := fsys.getSecurityByName(`\` + name)
		mustOK(t, "getSecurityByName", st)
		if attrs&mask != w {
			t.Errorf("getSecurityByName %s: attrs %#x, want %#x", name, attrs&mask, w)
		}
		fi, st := fsys.getDirInfoByName(root, name)
		mustOK(t, "getDirInfoByName", st)
		if fi.FileAttributes&mask != w {
			t.Errorf("getDirInfoByName %s: attrs %#x, want %#x", name, fi.FileAttributes&mask, w)
		}
		f, fi, st := fsys.open(`\`+name, 0)
		mustOK(t, "open", st)
		if fi.FileAttributes&mask != w {
			t.Errorf("open %s: attrs %#x, want %#x", name, fi.FileAttributes&mask, w)
		}
		fi, st = fsys.getFileInfo(f)
		mustOK(t, "getFileInfo", st)
		if fi.FileAttributes&mask != w {
			t.Errorf("getFileInfo %s: attrs %#x, want %#x", name, fi.FileAttributes&mask, w)
		}
		fsys.close(f)
	}
}

// TestDeletedSymlinkAttributes checks that a symlink that is deleted
// while open is not resolved against the root.
func TestDeletedSymlinkAttributes(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	f, _, st := fsys.create(`\link`, 0, 0, 0644)
	mustOK(t, "create", st)
	mustOK(t, "setSymlink", fsys.setSymlink(f, `\link`, "missing"))
	fsys.close(f)

	l, _, st := fsys.open(`\link`, 0)
	mustOK(t, "open", st)
	defer fsys.close(l)
	mustOK(t, "remove", fsys.remove(t, `\link`))
	fi, st := fsys.getFileInfo(l)
	mustOK(t, "getFileInfo", st)
	if fi.FileAttributes&_FILE_ATTRIBUTE_DIRECTORY != 0 {
		t.Errorf("attrs %#x, want no FILE_ATTRIBUTE_DIRECTORY", fi.FileAttributes)
	}
}

// TestShutdown checks that shutting down releases the handles that
// are still open, forgets all nodes, and calls OnUnmount.
func TestShutdown(t *testing.T) {
	c := &handleCounter{RawFileSystem: fs.NewNodeFS(memfs.NewRoot(), &fs.Options{})}
	fsys := newTestRawFS(t, c)
	f, _, st := fsys.create(`\f`, 0, 0, 0644)
	mustOK(t, "create", st)
	fsys.addHandle(f)
	d, _, st := fsys.open(`\`, _FILE_READ_DATA)
	mustOK(t, "open", st)
	fsys.addHandle(d)

	fsys.shutdown()
	if c.opens != c.releases || c.opens == 0 {
		t.Errorf("%d opens, %d releases", c.opens, c.releases)
	}
	if c.opendirs != c.relDirs || c.opendirs == 0 {
		t.Errorf("%d opendirs, %d releasedirs", c.opendirs, c.relDirs)
	}
	if !c.unmounted {
		t.Errorf("OnUnmount not called")
	}
	if l := fsys.tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}
}

// TestCreateReadOnlyDir checks that FILE_ATTRIBUTE_READONLY, which
// Windows ignores for directories, does not make a new directory
// unwritable.
func TestCreateReadOnlyDir(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, _, st := fsys.create(`\d`, _FILE_DIRECTORY_FILE, _FILE_ATTRIBUTE_READONLY, 0755)
	mustOK(t, "mkdir", st)
	a, st := fsys.getattr(d)
	fsys.close(d)
	mustOK(t, "getattr", st)
	if a.Mode&0200 == 0 {
		t.Errorf("mode %o, want owner write", a.Mode)
	}
	f, _, st := fsys.create(`\f`, 0, _FILE_ATTRIBUTE_READONLY, 0644)
	mustOK(t, "create", st)
	a, st = fsys.getattr(f)
	fsys.close(f)
	mustOK(t, "getattr", st)
	if a.Mode&0222 != 0 {
		t.Errorf("file mode %o, want read-only", a.Mode)
	}
}

// TestMkdirOpendirFails checks that a directory is removed if it
// cannot be opened after creating it.
func TestMkdirOpendirFails(t *testing.T) {
	root := memfs.NewRoot()
	fsys := newTestRawFS(t, &failOpendir{fs.NewNodeFS(root, &fs.Options{})})
	if _, _, st := fsys.create(`\d`, _FILE_DIRECTORY_FILE, 0, 0755); st == _STATUS_SUCCESS {
		t.Fatal("create succeeded")
	}
	if root.EmbeddedInode().GetChild("d") != nil {
		t.Errorf("directory exists after failed create")
	}
	if _, _, st := fsys.getSecurityByName(`\d`); st != _STATUS_OBJECT_NAME_NOT_FOUND {
		t.Errorf("getSecurityByName: %v", st)
	}
	if l := fsys.tree.Lookups(); len(l) != 0 {
		t.Errorf("lookups left: %v", l)
	}
}

// TestGetDirInfoByNameDots checks that "." and "..", which
// readDirectory lists, can also be looked up by name.
func TestGetDirInfoByNameDots(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	d, _, st := fsys.create(`\d`, _FILE_DIRECTORY_FILE, 0, 0755)
	mustOK(t, "mkdir", st)
	defer fsys.close(d)
	for _, name := range []string{".", ".."} {
		fi, st := fsys.getDirInfoByName(d, name)
		mustOK(t, name, st)
		if fi.FileAttributes&_FILE_ATTRIBUTE_DIRECTORY == 0 {
			t.Errorf("%s: attrs %#x", name, fi.FileAttributes)
		}
	}
	if _, st := fsys.getDirInfoByName(d, "missing"); st != _STATUS_OBJECT_NAME_NOT_FOUND {
		t.Errorf("missing: %v", st)
	}
}

// TestOverwriteAttributes checks that overwriting applies
// FILE_ATTRIBUTE_READONLY, and that replacing the attributes clears it.
func TestOverwriteAttributes(t *testing.T) {
	fsys := newTestFS(t, nil, nil)
	fsys.createFile(t, `\f`, []byte("data"))
	for _, tc := range []struct {
		attrs    uint32
		replace  bool
		readOnly bool
	}{
		{_FILE_ATTRIBUTE_READONLY, false, true},
		{0, false, true},
		{_FILE_ATTRIBUTE_NORMAL, true, false},
	} {
		f, _, st := fsys.open(`\f`, _FILE_WRITE_DATA)
		mustOK(t, "open", st)
		fi, st := fsys.overwrite(f, tc.attrs, tc.replace)
		fsys.close(f)
		mustOK(t, "overwrite", st)
		if fi.FileSize != 0 {
			t.Errorf("%+v: size %d", tc, fi.FileSize)
		}
		if got := fi.FileAttributes&_FILE_ATTRIBUTE_READONLY != 0; got != tc.readOnly {
			t.Errorf("%+v: attrs %#x", tc, fi.FileAttributes)
		}
	}
}

// setattrRecorder records the SETATTR requests.
type setattrRecorder struct {
	fuse.RawFileSystem

	mu  sync.Mutex
	ins []fuse.SetAttrIn
}

func (r *setattrRecorder) SetAttr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	r.mu.Lock()
	r.ins = append(r.ins, *in)
	r.mu.Unlock()
	return r.RawFileSystem.SetAttr(ctx, in, out)
}

// TestSetBasicInfoSymlink checks that FILE_ATTRIBUTE_READONLY on a
// symlink, which a POSIX symlink cannot store, is ignored, while the
// timestamps are still set.
func TestSetBasicInfoSymlink(t *testing.T) {
	r := &setattrRecorder{RawFileSystem: fs.NewNodeFS(memfs.NewRoot(), &fs.Options{})}
	fsys := newTestRawFS(t, r)
	fsys.createFile(t, `\f`, []byte("data"))
	l, _, st := fsys.create(`\link`, 0, 0, 0644)
	mustOK(t, "create", st)
	mustOK(t, "setSymlink", fsys.setSymlink(l, `\link`, "f"))
	fsys.close(l)

	f, _, st := fsys.open(`\link`, 0)
	mustOK(t, "open", st)
	defer fsys.close(f)
	if !f.isLink {
		t.Fatal("not opened as a symlink")
	}
	r.ins = nil
	mtime := filetime(1e9, 0)
	fi, st := fsys.setBasicInfo(f, _FILE_ATTRIBUTE_READONLY, 0, 0, mtime, 0)
	mustOK(t, "setBasicInfo", st)
	if fi.FileAttributes&_FILE_ATTRIBUTE_READONLY != 0 {
		t.Errorf("attrs %#x, want not read-only", fi.FileAttributes)
	}
	if len(r.ins) != 1 {
		t.Fatalf("got %d SETATTRs, want 1", len(r.ins))
	}
	in := r.ins[0]
	if in.Valid&fuse.FATTR_MODE != 0 {
		t.Errorf("SETATTR changes the mode of a symlink: %o", in.Mode)
	}
	if in.Valid&fuse.FATTR_MTIME == 0 || in.Mtime != 1e9 {
		t.Errorf("SETATTR %+v, want mtime 1e9", in)
	}
	if a, _ := fsys.getattr(f); a.Mode&07777 != 0777 {
		t.Errorf("symlink mode %o", a.Mode)
	}
}

// getattrCounter counts GETATTR requests.
type getattrCounter struct {
	fuse.RawFileSystem

	mu sync.Mutex
	n  int
}

func (c *getattrCounter) GetAttr(ctx context.Context, in *fuse.GetAttrIn, out *fuse.AttrOut) syscall.Errno {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return c.RawFileSystem.GetAttr(ctx, in, out)
}

// TestWriteGetattrs checks that a write sends one GETATTR, and
// reports the new size.
func TestWriteGetattrs(t *testing.T) {
	c := &getattrCounter{RawFileSystem: fs.NewNodeFS(memfs.NewRoot(), &fs.Options{})}
	fsys := newTestRawFS(t, c)
	fsys.createFile(t, `\f`, []byte("hello"))
	f, _, st := fsys.open(`\f`, _FILE_READ_DATA|_FILE_WRITE_DATA)
	mustOK(t, "open", st)
	defer fsys.close(f)

	for _, tc := range []struct {
		name        string
		data        string
		off         uint64
		toEnd       bool
		constrained bool
		wantN       int
		wantSize    uint64
	}{
		{"overwrite", "HE", 0, false, false, 2, 5},
		{"extend", "xyz", 4, false, false, 3, 7},
		{"append", "!", 0, true, false, 1, 8},
		{"constrained", "abcdef", 6, false, true, 2, 8},
	} {
		c.mu.Lock()
		c.n = 0
		c.mu.Unlock()
		n, fi, st := fsys.write(f, []byte(tc.data), tc.off, tc.toEnd, tc.constrained)
		mustOK(t, tc.name, st)
		if n != tc.wantN || fi.FileSize != tc.wantSize {
			t.Errorf("%s: wrote %d, size %d; want %d, %d", tc.name, n, fi.FileSize, tc.wantN, tc.wantSize)
		}
		if c.n != 1 {
			t.Errorf("%s: %d GETATTRs, want 1", tc.name, c.n)
		}
	}
	buf := make([]byte, 100)
	n, st := fsys.read(f, buf, 0)
	mustOK(t, "read", st)
	if got, want := string(buf[:n]), "HEllxyab"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
