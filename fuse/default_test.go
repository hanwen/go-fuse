package fuse

// Make sure library supplied Filesystems support the
// required interface.

import (
	"testing"
)

func TestRawFs(t *testing.T) {
	var iface RawFileSystem

	iface = new(DefaultRawFuseFileSystem)
	iface = new(WrappingRawFilesystem)
	iface = new(TimingRawFilesystem)

	_ = iface
}

func TestPathFs(t *testing.T) {
	var iface PathFilesystem
	iface = new(DefaultPathFilesystem)
	iface = new(WrappingPathFilesystem)
	iface = new(TimingPathFilesystem)

	_ = iface
}

func TestDummyFile(t *testing.T) {
	d := new(DefaultRawFuseFile)
	var filePtr RawFuseFile = d
	_ = filePtr
}
