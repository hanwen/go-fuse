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

func setupCacheTest() (string, func()) {
	dir := MakeTempDir()
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := &cacheFs{
		LoopbackFileSystem: NewLoopbackFileSystem(dir + "/orig"),
	}
	state, _, err := MountFileSystem(dir+"/mnt", fs, nil)
	CheckSuccess(err)

	go state.Loop(false)

	return dir, func() {
		err := state.Unmount()
		if err == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestCacheFs(t *testing.T) {
	wd, clean := setupCacheTest()
	defer clean()

	err := ioutil.WriteFile(wd+"/orig/file.txt", []byte("hello"), 0644)
	CheckSuccess(err)

	c, err := ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)

	if string(c) != "hello" {
		t.Fatalf("expect 'hello' %q", string(c))
	}

	err = ioutil.WriteFile(wd+"/orig/file.txt", []byte("qqqqq"), 0644)
	CheckSuccess(err)

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	CheckSuccess(err)

	if string(c) != "hello" {
		t.Fatalf("expect 'hello' %q", string(c))
	}
}
