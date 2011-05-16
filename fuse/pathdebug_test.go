package fuse

import (
	"io/ioutil"
	"path/filepath"
	"os"
	"testing"
)

func TestPathDebug(t *testing.T) {
	debugFs := NewFileSystemDebug()
	debugFs.FileSystem = &DefaultFileSystem{}
	debugFs.Add("test-entry", func() []byte { return []byte("test-content") })

	connector := NewFileSystemConnector(debugFs, nil)
	mountPoint := MakeTempDir()
	defer os.RemoveAll(mountPoint)

	state := NewMountState(connector)
	state.Mount(mountPoint, nil)
	state.Debug = true
	defer state.Unmount()

	go state.Loop(false)

	dir := filepath.Join(mountPoint, ".debug")
	_, err := os.Lstat(dir)
	CheckSuccess(err)

	names, err := ioutil.ReadDir(dir)
	CheckSuccess(err)

	if len(names) != 1 || names[0].Name != "test-entry" {
		t.Error("unexpected readdir out:", names)
	}

	c, err := ioutil.ReadFile(filepath.Join(dir, "test-entry"))
	CheckSuccess(err)

	if string(c) != "test-content" {
		t.Error("unexpected content", c)
	}
}
