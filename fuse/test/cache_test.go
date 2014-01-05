package test

import (
	"bytes"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

type cacheFs struct {
	pathfs.FileSystem
}

func (fs *cacheFs) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	f, c := fs.FileSystem.Open(name, flags, context)
	if !c.Ok() {
		return f, c
	}
	return &nodefs.WithFlags{
		File:      f,
		FuseFlags: fuse.FOPEN_KEEP_CACHE,
	}, c

}

func setupCacheTest(t *testing.T) (string, *pathfs.PathNodeFs, func()) {
	dir, err := ioutil.TempDir("", "go-fuse-cachetest")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := &cacheFs{
		pathfs.NewLoopbackFileSystem(dir + "/orig"),
	}
	pfs := pathfs.NewPathNodeFs(fs, nil)
	state, conn, err := nodefs.MountRoot(dir+"/mnt", pfs.Root(), nil)
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	state.SetDebug(VerboseTest())
	conn.SetDebug(VerboseTest())
	pfs.SetDebug(VerboseTest())
	go state.Serve()

	return dir, pfs, func() {
		err := state.Unmount()
		if err == nil {
			os.RemoveAll(dir)
		}
	}
}

func TestCacheFs(t *testing.T) {
	wd, pathfs, clean := setupCacheTest(t)
	defer clean()

	content1 := "hello"
	content2 := "qqqq"
	err := ioutil.WriteFile(wd+"/orig/file.txt", []byte(content1), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	c, err := ioutil.ReadFile(wd + "/mnt/file.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(c) != "hello" {
		t.Fatalf("expect 'hello' %q", string(c))
	}

	err = ioutil.WriteFile(wd+"/orig/file.txt", []byte(content2), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(c) != "hello" {
		t.Fatalf("Page cache skipped: expect 'hello' %q", string(c))
	}

	code := pathfs.EntryNotify("", "file.txt")
	if !code.Ok() {
		t.Errorf("Entry notify failed: %v", code)
	}

	c, err = ioutil.ReadFile(wd + "/mnt/file.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(c) != string(content2) {
		t.Fatalf("Mismatch after notify expect '%s' %q", content2, string(c))
	}
}

type nonseekFs struct {
	pathfs.FileSystem
	Length int
}

func (fs *nonseekFs) GetAttr(name string, context *fuse.Context) (fi *fuse.Attr, status fuse.Status) {
	if name == "file" {
		return &fuse.Attr{Mode: fuse.S_IFREG | 0644}, fuse.OK
	}
	return nil, fuse.ENOENT
}

func (fs *nonseekFs) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	if name != "file" {
		return nil, fuse.ENOENT
	}

	data := bytes.Repeat([]byte{42}, fs.Length)
	f := nodefs.NewDataFile(data)
	return &nodefs.WithFlags{
		File:      f,
		FuseFlags: fuse.FOPEN_NONSEEKABLE,
	}, fuse.OK
}

func TestNonseekable(t *testing.T) {
	fs := &nonseekFs{FileSystem: pathfs.NewDefaultFileSystem()}
	fs.Length = 200 * 1024

	dir, err := ioutil.TempDir("", "go-fuse-cache_test")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer os.RemoveAll(dir)
	nfs := pathfs.NewPathNodeFs(fs, nil)
	state, _, err := nodefs.MountRoot(dir, nfs.Root(), nil)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	state.SetDebug(VerboseTest())
	defer state.Unmount()

	go state.Serve()

	f, err := os.Open(dir + "/file")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer f.Close()

	b := make([]byte, 200)
	n, err := f.ReadAt(b, 20)
	if err == nil || n > 0 {
		t.Errorf("file was opened nonseekable, but seek successful")
	}
}

func TestGetAttrRace(t *testing.T) {
	dir, err := ioutil.TempDir("", "go-fuse-cache_test")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer os.RemoveAll(dir)
	os.Mkdir(dir+"/mnt", 0755)
	os.Mkdir(dir+"/orig", 0755)

	fs := pathfs.NewLoopbackFileSystem(dir + "/orig")
	pfs := pathfs.NewPathNodeFs(fs, nil)
	state, conn, err := nodefs.MountRoot(dir+"/mnt", pfs.Root(),
		&nodefs.Options{})
	if err != nil {
		t.Fatalf("MountNodeFileSystem failed: %v", err)
	}
	state.SetDebug(VerboseTest())
	conn.SetDebug(VerboseTest())
	pfs.SetDebug(VerboseTest())
	go state.Serve()

	defer state.Unmount()

	var wg sync.WaitGroup

	n := 100
	wg.Add(n)
	var statErr error
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
