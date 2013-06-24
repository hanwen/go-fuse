package test

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

var _ = log.Println

type NotifyFs struct {
	pathfs.FileSystem
	size  uint64
	exist bool
}

func (fs *NotifyFs) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	if name == "" {
		return &fuse.Attr{Mode: fuse.S_IFDIR | 0755}, fuse.OK
	}
	if name == "file" || (name == "dir/file" && fs.exist) {
		return &fuse.Attr{Mode: fuse.S_IFREG | 0644, Size: fs.size}, fuse.OK
	}
	if name == "dir" {
		return &fuse.Attr{Mode: fuse.S_IFDIR | 0755}, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (fs *NotifyFs) Open(name string, f uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	return nodefs.NewDataFile([]byte{42}), fuse.OK
}

type NotifyTest struct {
	fs        *NotifyFs
	pathfs    *pathfs.PathNodeFs
	connector *nodefs.FileSystemConnector
	dir       string
	state     *fuse.Server
}

func NewNotifyTest(t *testing.T) *NotifyTest {
	me := &NotifyTest{}
	me.fs = &NotifyFs{FileSystem: pathfs.NewDefaultFileSystem()}
	var err error
	me.dir, err = ioutil.TempDir("", "go-fuse-notify_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	entryTtl := 100 * time.Millisecond
	opts := &nodefs.Options{
		EntryTimeout:    entryTtl,
		AttrTimeout:     entryTtl,
		NegativeTimeout: entryTtl,
	}

	me.pathfs = pathfs.NewPathNodeFs(me.fs, nil)
	me.state, me.connector, err = nodefs.MountFileSystem(me.dir, me.pathfs, opts)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	me.state.SetDebug(fuse.VerboseTest())
	go me.state.Serve()

	return me
}

func (t *NotifyTest) Clean() {
	err := t.state.Unmount()
	if err == nil {
		os.RemoveAll(t.dir)
	}
}

func TestInodeNotify(t *testing.T) {
	test := NewNotifyTest(t)
	defer test.Clean()

	fs := test.fs
	dir := test.dir

	fs.size = 42
	test.state.ThreadSanitizerSync()

	fi, err := os.Lstat(dir + "/file")
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if fi.Mode()&os.ModeType != 0 || fi.Size() != 42 {
		t.Error(fi)
	}

	test.state.ThreadSanitizerSync()
	fs.size = 666

	fi, err = os.Lstat(dir + "/file")
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if fi.Mode()&os.ModeType != 0 || fi.Size() == 666 {
		t.Error(fi)
	}

	code := test.pathfs.FileNotify("file", -1, 0)
	if !code.Ok() {
		t.Error(code)
	}

	fi, err = os.Lstat(dir + "/file")
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
	if fi.Mode()&os.ModeType != 0 || fi.Size() != 666 {
		t.Error(fi)
	}
}

func TestEntryNotify(t *testing.T) {
	test := NewNotifyTest(t)
	defer test.Clean()

	dir := test.dir
	test.fs.size = 42
	test.fs.exist = false
	test.state.ThreadSanitizerSync()

	fn := dir + "/dir/file"
	fi, _ := os.Lstat(fn)
	if fi != nil {
		t.Errorf("File should not exist, %#v", fi)
	}

	test.fs.exist = true
	test.state.ThreadSanitizerSync()
	fi, _ = os.Lstat(fn)
	if fi != nil {
		t.Errorf("negative entry should have been cached: %#v", fi)
	}

	code := test.pathfs.EntryNotify("dir", "file")
	if !code.Ok() {
		t.Errorf("EntryNotify returns error: %v", code)
	}

	fi, err := os.Lstat(fn)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}
}
