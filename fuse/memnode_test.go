package fuse

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
)

var _ = log.Println

func setupMemNodeTest(t *testing.T) (wd string, fs *MemNodeFs, clean func()) {
	tmp, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	back := tmp + "/backing"
	os.Mkdir(back, 0700)
	fs = NewMemNodeFs(back)
	mnt := tmp + "/mnt"
	os.Mkdir(mnt, 0700)

	connector := NewFileSystemConnector(fs,
		&FileSystemOptions{
			EntryTimeout:    testTtl,
			AttrTimeout:     testTtl,
			NegativeTimeout: 0.0,
		})
	connector.Debug = VerboseTest()
	state := NewMountState(connector)
	state.Mount(mnt, nil)

	//me.state.Debug = false
	state.Debug = VerboseTest()

	// Unthreaded, but in background.
	go state.Loop()
	return mnt, fs, func() {
		state.Unmount()
		os.RemoveAll(tmp)
	}

}

func TestMemNodeFs(t *testing.T) {
	wd, _, clean := setupMemNodeTest(t)
	defer clean()

	err := ioutil.WriteFile(wd+"/test", []byte{42}, 0644)
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/test")
	CheckSuccess(err)
	if fi.Size != 1 {
		t.Errorf("Size after write incorrect: got %d want 1", fi.Size)
	}

	entries, err := ioutil.ReadDir(wd)
	if len(entries) != 1 || entries[0].Name != "test" {
		t.Fatalf("Readdir got %v, expected 1 file named 'test'", entries)
	}
}

func TestMemNodeSetattr(t *testing.T) {
	wd, _, clean := setupMemNodeTest(t)
	defer clean()

	f, err := os.OpenFile(wd+"/test", os.O_CREATE|os.O_WRONLY, 0644)
	CheckSuccess(err)
	defer f.Close()

	err = f.Truncate(4096)
	CheckSuccess(err)

	fi, err := f.Stat()
	CheckSuccess(err)
	if fi.Size != 4096 {
		t.Errorf("Size should be 4096 after Truncate: %d", fi.Size)
	}
}
