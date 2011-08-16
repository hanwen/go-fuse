package fuse

import (
	"testing"
	"os"
	"io/ioutil"
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
