package fuse

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

type cacheFs struct {
	*LoopbackFileSystem
}

func (me *cacheFs) Open(name string, flags uint32) (fuseFile File, status Status) {
	f, c := me.LoopbackFileSystem.Open(name, flags)
	if !c.Ok() {
		return f, c
	}
	return &WithFlags{
		File:  f,
		Flags: FOPEN_KEEP_CACHE,
	}, c

}

func setupCacheTest() (string, *FileSystemConnector, func()) {
	dir := MakeTempDir()
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := &cacheFs{
		LoopbackFileSystem: NewLoopbackFileSystem(dir + "/orig"),
	}
	state, conn, err := MountFileSystem(dir+"/mnt", fs, nil)
	CheckSuccess(err)
	state.Debug = true
	conn.Debug = true
	go state.Loop(false)

	return dir, conn, func() {
		err := state.Unmount()
		if err == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestCacheFs(t *testing.T) {
	wd, conn, clean := setupCacheTest()
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
		t.Fatalf("expect 'hello' %q", string(c))
	}

	code := conn.EntryNotify("", "file.txt")
	if !code.Ok() {
		t.Errorf("Entry notify failed: %v", code)
	}

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)
	if string(c) != string(content2) {
		t.Fatalf("expect '%s' %q", content2, string(c))
	}
}

type nonseekFs struct {
	DefaultFileSystem
	Length int 
}

func (me *nonseekFs) GetAttr(name string) (fi *os.FileInfo, status Status) {
	if name == "file" {
		return &os.FileInfo{ Mode: S_IFREG | 0644 }, OK
	}
	return nil, ENOENT
}

func (me *nonseekFs) Open(name string, flags uint32) (fuseFile File, status Status) {
	if name != "file" {
		return nil, ENOENT
	}

	data := bytes.Repeat([]byte{42}, me.Length)
	f := NewReadOnlyFile(data)
	return &WithFlags{
		File:  f,
		Flags: FOPEN_NONSEEKABLE,
	}, OK
}

func TestNonseekable(t *testing.T) {
	fs := &nonseekFs{}
	fs.Length = 200*1024

	dir := MakeTempDir()
	defer os.RemoveAll(dir)
	state, _, err := MountFileSystem(dir, fs, nil)
	CheckSuccess(err)
	state.Debug = true
	defer state.Unmount()

	go state.Loop(false)

	f, err := os.Open(dir + "/file")
	CheckSuccess(err)
	defer f.Close()
	
	b := make([]byte, 200)
	n, err := f.ReadAt(b, 20)
	if err == nil || n > 0 {
		t.Errorf("file was opened nonseekable, but seek successful")
	}
}
