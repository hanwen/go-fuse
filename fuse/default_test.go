package fuse

// Make sure library supplied FileSystems support the
// required interface.

import (
	"testing"
)

func TestRawFs(t *testing.T) {
	var iface RawFileSystem

	iface = new(DefaultRawFuseFileSystem)
	iface = new(WrappingRawFileSystem)
	iface = new(TimingRawFileSystem)

	_ = iface
}

func TestPathFs(t *testing.T) {
	var iface PathFileSystem
	iface = new(DefaultPathFileSystem)
	iface = new(WrappingPathFileSystem)
	iface = new(TimingPathFileSystem)

	_ = iface
}

func TestDummyFile(t *testing.T) {
	d := new(DefaultFuseFile)
	var filePtr FuseFile = d
	_ = filePtr
}
