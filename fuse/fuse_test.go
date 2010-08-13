package fuse

import (
	"log"
	"os"
	"testing"
)

const (
	tempMountDir = "./.testMountDir"
)

type testFuse struct{}

func (fs *testFuse) GetAttr(h *InHeader, in *GetAttrIn) (out *AttrOut, code Error, err os.Error) {
	out = new(AttrOut)
	out.Ino = h.NodeId
	out.Mode = S_IFDIR
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
