package fuse

// Make sure library supplied FileSystems support the
// required interface.

import (
	"testing"
)

func TestRawFs(t *testing.T) {
	var iface RawFileSystem

	iface = new(DefaultRawFileSystem)
	iface = new(WrappingRawFileSystem)
	iface = new(TimingRawFileSystem)

	_ = iface
}

func TestPathFs(t *testing.T) {
	var iface FileSystem
	iface = new(DefaultFileSystem)
	iface = new(WrappingFileSystem)
	iface = new(TimingFileSystem)

	_ = iface
}

func TestDummyFile(t *testing.T) {
	d := new(DefaultFile)
	var filePtr File = d
	_ = filePtr
}
