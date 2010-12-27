package fuse

// Compilation test for DummyFuse and DummyPathFuse

import (
	"testing"
)

func TestDummy(t *testing.T) {
	fs := new(DummyFuse)
	NewMountState(fs)

	pathFs := new(DummyPathFuse)

	NewPathFileSystemConnector(pathFs)
}
