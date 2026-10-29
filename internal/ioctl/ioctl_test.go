//go:build linux

package ioctl

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestKernelEncoding(t *testing.T) {
	// sizeof(long) on Linux.
	sz := unsafe.Sizeof(uintptr(0))

	// FS_IOC_GETFLAGS = _IOR('f', 1, long): userspace reads.
	if got := New(READ, 'f', 1, sz); uint32(got) != uint32(unix.FS_IOC_GETFLAGS) {
		t.Errorf("New(READ, 'f', 1, %d) = %#x, want FS_IOC_GETFLAGS %#x", sz, got, uint32(unix.FS_IOC_GETFLAGS))
	}
	c := Command(unix.FS_IOC_GETFLAGS)
	if !c.Read() || c.Write() {
		t.Errorf("FS_IOC_GETFLAGS: Read()=%v Write()=%v, want true false", c.Read(), c.Write())
	}

	// FS_IOC_SETFLAGS = _IOW('f', 2, long): userspace writes.
	c = Command(unix.FS_IOC_SETFLAGS)
	if c.Read() || !c.Write() {
		t.Errorf("FS_IOC_SETFLAGS: Read()=%v Write()=%v, want false true", c.Read(), c.Write())
	}
}
