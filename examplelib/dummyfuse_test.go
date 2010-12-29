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
