package fuse

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
)

var _ = log.Println

type NotifyFs struct {
	DefaultFileSystem
	size  int64
	exist bool
}

func (me *NotifyFs) GetAttr(name string, context *Context) (*os.FileInfo, Status) {
	if name == "" {
		return &os.FileInfo{Mode: S_IFDIR | 0755}, OK
	}
	if name == "file" || (name == "dir/file" && me.exist) {
		return &os.FileInfo{Mode: S_IFREG | 0644, Size: me.size}, OK
	}
	if name == "dir" {
		return &os.FileInfo{Mode: S_IFDIR | 0755}, OK
	}
	return nil, ENOENT
}

func (me *NotifyFs) Open(name string, f uint32, context *Context) (File, Status) {
	return NewDataFile([]byte{42}), OK
}

type NotifyTest struct {
	fs        *NotifyFs
	pathfs    *PathNodeFs
	connector *FileSystemConnector
	dir       string
	state     *MountState
}

func NewNotifyTest() *NotifyTest {
	me := &NotifyTest{}
	me.fs = &NotifyFs{}
	var err os.Error
	me.dir, err = ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	entryTtl := 0.1
	opts := &FileSystemOptions{
		EntryTimeout:    entryTtl,
		AttrTimeout:     entryTtl,
		NegativeTimeout: entryTtl,
	}

	me.pathfs = NewPathNodeFs(me.fs, nil)
	me.state, me.connector, err = MountNodeFileSystem(me.dir, me.pathfs, opts)
	CheckSuccess(err)
	me.state.Debug = VerboseTest()
	go me.state.Loop()

	return me
}

func (me *NotifyTest) Clean() {
	err := me.state.Unmount()
	if err == nil {
		os.RemoveAll(me.dir)
	}
}

func TestInodeNotify(t *testing.T) {
	test := NewNotifyTest()
	defer test.Clean()

	fs := test.fs
	dir := test.dir

	fs.size = 42
	fi, err := os.Lstat(dir + "/file")
	CheckSuccess(err)
	if !fi.IsRegular() || fi.Size != 42 {
		t.Error(fi)
	}

	fs.size = 666
	fi, err = os.Lstat(dir + "/file")
	CheckSuccess(err)
	if !fi.IsRegular() || fi.Size == 666 {
		t.Error(fi)
	}

	code := test.pathfs.FileNotify("file", -1, 0)
	if !code.Ok() {
		t.Error(code)
	}

	fi, err = os.Lstat(dir + "/file")
	CheckSuccess(err)
	if !fi.IsRegular() || fi.Size != 666 {
		t.Error(fi)
	}
}

func TestEntryNotify(t *testing.T) {
	test := NewNotifyTest()
	defer test.Clean()

	dir := test.dir
	test.fs.size = 42
	test.fs.exist = false
	fn := dir + "/dir/file"
	fi, _ := os.Lstat(fn)
	if fi != nil {
		t.Errorf("File should not exist, %#v", fi)
	}

	test.fs.exist = true
	fi, _ = os.Lstat(fn)
	if fi != nil {
		t.Errorf("negative entry should have been cached: %#v", fi)
	}

	code := test.pathfs.EntryNotify("dir", "file")
	if !code.Ok() {
		t.Errorf("EntryNotify returns error: %v", code)
	}

	fi, err := os.Lstat(fn)
	CheckSuccess(err)
}
