package fuse

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"testing"
)

var _ = log.Println

type cacheFs struct {
	*LoopbackFileSystem
}

func (me *cacheFs) Open(name string, flags uint32, context *Context) (fuseFile File, status Status) {
	f, c := me.LoopbackFileSystem.Open(name, flags, context)
	if !c.Ok() {
		return f, c
	}
	return &WithFlags{
		File:      f,
		FuseFlags: FOPEN_KEEP_CACHE,
	}, c

}

func setupCacheTest() (string, *PathNodeFs, func()) {
	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := &cacheFs{
		LoopbackFileSystem: NewLoopbackFileSystem(dir + "/orig"),
	}
	pfs := NewPathNodeFs(fs, nil)
	state, conn, err := MountNodeFileSystem(dir+"/mnt", pfs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()
	conn.Debug = VerboseTest()
	pfs.Debug = VerboseTest()
	go state.Loop()

	return dir, pfs, func() {
		err := state.Unmount()
		if err == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestCacheFs(t *testing.T) {
	wd, pathfs, clean := setupCacheTest()
	defer clean()

	content1 := "hello"
	content2 := "qqqq"
	err := ioutil.WriteFile(wd+"/orig/file.txt", []byte(content1), 0644)
	CheckSuccess(err)

	c, err := ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)

	if string(c) != "hello" {
		t.Fatalf("expect 'hello' %q", string(c))
	}

	err = ioutil.WriteFile(wd+"/orig/file.txt", []byte(content2), 0644)
	CheckSuccess(err)

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)

	if string(c) != "hello" {
		t.Fatalf("Page cache skipped: expect 'hello' %q", string(c))
	}

	code := pathfs.EntryNotify("", "file.txt")
	if !code.Ok() {
		t.Errorf("Entry notify failed: %v", code)
	}

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)
	if string(c) != string(content2) {
		t.Fatalf("Mismatch after notify expect '%s' %q", content2, string(c))
	}
}

type nonseekFs struct {
	DefaultFileSystem
	Length int
}

func (me *nonseekFs) GetAttr(name string, context *Context) (fi *os.FileInfo, status Status) {
	if name == "file" {
		return &os.FileInfo{Mode: S_IFREG | 0644}, OK
	}
	return nil, ENOENT
}

func (me *nonseekFs) Open(name string, flags uint32, context *Context) (fuseFile File, status Status) {
	if name != "file" {
		return nil, ENOENT
	}

	data := bytes.Repeat([]byte{42}, me.Length)
	f := NewDataFile(data)
	return &WithFlags{
		File:      f,
		FuseFlags: FOPEN_NONSEEKABLE,
	}, OK
}

func TestNonseekable(t *testing.T) {
	fs := &nonseekFs{}
	fs.Length = 200 * 1024

	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(dir)
	state, _, err := MountPathFileSystem(dir, fs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()
	defer state.Unmount()

	go state.Loop()

	f, err := os.Open(dir + "/file")
	CheckSuccess(err)
	defer f.Close()

	b := make([]byte, 200)
	n, err := f.ReadAt(b, 20)
	if err == nil || n > 0 {
		t.Errorf("file was opened nonseekable, but seek successful")
	}
}

func TestGetAttrRace(t *testing.T) {
	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	defer os.RemoveAll(dir)
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := NewLoopbackFileSystem(dir + "/orig")
	pfs := NewPathNodeFs(fs, nil)
	state, conn, err := MountNodeFileSystem(dir+"/mnt", pfs,
		&FileSystemOptions{})
	CheckSuccess(err)
	state.Debug = VerboseTest()
	conn.Debug = VerboseTest()
	pfs.Debug = VerboseTest()
	go state.Loop()

	defer state.Unmount()

	var wg sync.WaitGroup

	n := 100
	wg.Add(n)
	var statErr os.Error
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			fn := dir + "/mnt/file"
			err := ioutil.WriteFile(fn, []byte{42}, 0644)
			if err != nil {
				statErr = err
				return
			}
			_, err = os.Lstat(fn)
			if err != nil {
				statErr = err
			}
		}()
	}
	wg.Wait()
	if statErr != nil {
		t.Error(statErr)
	}
}
