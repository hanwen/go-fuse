package examplelib

// Compilation test for DummyFuse and DummyPathFuse

import (
	"github.com/hanwen/go-fuse/fuse"
	"testing"
)

func TestDummy(t *testing.T) {
	fs := new(DummyFuse)
	fuse.NewMountState(fs)

	pathFs := new(DummyPathFuse)

	fuse.NewPathFileSystemConnector(pathFs)
}

func TestDummyFile(t *testing.T) {
	d := new(DummyFuseFile)
	var filePtr fuse.RawFuseFile = d
	var fileDir fuse.RawFuseDir = d
	_ = fileDir
	_ = filePtr
}
