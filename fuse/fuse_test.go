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

func (fs *testFuse) GetAttr(path string) (out Attr, code Status) {
	if strings.HasSuffix(path, ".txt") {
		out.Mode = S_IFREG + 0644
	} else {
		out.Mode = S_IFDIR + 0755
	}
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
