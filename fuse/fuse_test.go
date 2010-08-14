package fuse

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	tempMountDir = "testMountDir"
)

var (
	testFileNames = []string{"one", "two", "three3"}
)

type testFuse struct{}

func (fs *testFuse) GetAttr(path string) (out *Attr, code Status) {
	out = new(Attr)
	out.Mode = S_IFDIR
	out.Mtime = uint64(time.Seconds())
	return
}

func (fs *testFuse) List(dir string) (names []string, code Status) {
	names = testFileNames
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
	defer func() {
		err := m.Unmount()
		if err != nil {
			t.Fatalf("Can't unmount a dir, err: %v", err)
		}
	}()
	go errorHandler(errors)
	f, err := os.Open(tempMountDir, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Can't open a dir: %s, err: %v", tempMountDir, err)
	}
	defer f.Close()
	names, err := f.Readdirnames(10)
	if err != nil {
		t.Fatalf("Can't ls a dir: %s, err: %v", tempMountDir, err)
	}
	has := strings.Join(names, ", ")
	wanted := strings.Join(testFileNames, ", ")
	if has != wanted {
		t.Errorf("Ls returned wrong results, has: [%s], wanted: [%s]", has, wanted)
		return
	}
}
