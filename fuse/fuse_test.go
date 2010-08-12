package fuse

import (
	"fmt"
	"log"
	"os"
	"testing"
)

const (
	tempMountDir = "./.testMountDir"
)

type testFuse struct{}

func (fs *testFuse) Init(in *InitIn) (out *InitOut, code Error) {
	if in.Major != FUSE_KERNEL_VERSION {
		fmt.Printf("Major versions does not match. Given %d, want %d\n", in.Major, FUSE_KERNEL_VERSION)
		code = EIO
		return
	}
	if in.Minor < FUSE_KERNEL_MINOR_VERSION {
		fmt.Printf("Minor version is less than we support. Given %d, want at least %d\n", in.Minor, FUSE_KERNEL_MINOR_VERSION)
		code = EIO
		return
	}
	out = new(InitOut)
	out.Major = FUSE_KERNEL_VERSION
	out.Minor = FUSE_KERNEL_MINOR_VERSION
	out.MaxReadAhead = in.MaxReadAhead
	out.MaxWrite = 65536
	return
}

func errorHandler(errors chan os.Error) {
	for err := range errors {
		log.Stderr("MountPoint.errorHandler: ", err)
		if err == os.EOF {
			break
		}
	}
}


func TestMount(t *testing.T) {
	fs := new(testFuse)
	//	toW := make(chan [][]byte, 100)
	//	errors := make(chan os.Error, 100)

	//	in_data := []byte { 56, 0, 0, 0, 26, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 13, 0, 0, 0, 0, 0, 2, 0, 123, 0, 0, 0, }
	//	handle(fs, in_data, toW, errors)
	//	return

	err := os.Mkdir(tempMountDir, 0777)
	if err != nil {
		t.Fatalf("Can't create temp mount dir at %s, err: %v", tempMountDir, err)
	}
	defer os.Remove(tempMountDir)
	m, err, errors := Mount(tempMountDir, fs)
	if err != nil {
		t.Fatalf("Can't mount a dir, err: %v", err)
	}
	errorHandler(errors)
	err = m.Unmount()
	if err != nil {
		t.Fatalf("Can't unmount a dir, err: %v", err)
	}
}
