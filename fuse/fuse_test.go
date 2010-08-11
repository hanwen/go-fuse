package fuse

import (
	"os"
	"testing"
)

const (
	tempMountDir = "./.testMountDir"
)

type testFuse struct {}

func TestMount(t *testing.T) {
	fs := new(testFuse)
	err := os.Mkdir(tempMountDir, 0777)
	if err != nil {
		t.Fatalf("Can't create temp mount dir at %s, err: %v", tempMountDir, err)
	}
	defer os.Remove(tempMountDir)
	m, err := Mount(tempMountDir, fs)
	if err != nil {
		t.Fatal("Can't mount a dir, err: %v", err)
	}
	err = m.Unmount()
	if err != nil {
		t.Fatal("Can't unmount a dir, err: %v", err)
	}
}

