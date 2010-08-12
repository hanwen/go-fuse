package fuse

import (
	"fmt"
	"os"
	"testing"
)

const (
	tempMountDir = "./.testMountDir"
)

type testFuse struct {}

func (fs *testFuse) Init(in *InitIn) (out *InitOut, code Error) {
	if (in.Major != FUSE_KERNEL_VERSION) {
		fmt.Printf("Major versions does not match. Given %d, want %d\n", in.Major, FUSE_KERNEL_VERSION)
		return nil, EIO
	}
	if (in.Minor < FUSE_KERNEL_MINOR_VERSION) {
		fmt.Printf("Minor version is less than we support. Given %d, want at least %d\n", in.Minor, FUSE_KERNEL_MINOR_VERSION)
		return nil, EIO
	}
	out.Major = FUSE_KERNEL_VERSION
	out.Minor = FUSE_KERNEL_MINOR_VERSION
	out.MaxReadahead = 65536
	out.MaxWrite = 65536
	return
}

func TestMount(t *testing.T) {
	fs := new(testFuse)
	err := os.Mkdir(tempMountDir, 0777)
	if err != nil {
		t.Fatalf("Can't create temp mount dir at %s, err: %v", tempMountDir, err)
	}
	defer os.Remove(tempMountDir)
	m, err := Mount(tempMountDir, fs)
	if err != nil {
		t.Fatalf("Can't mount a dir, err: %v", err)
	}
	err = m.Unmount()
	if err != nil {
		t.Fatalf("Can't unmount a dir, err: %v", err)
	}
}

