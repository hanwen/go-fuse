package fuse

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
)

var _ = log.Println

type DefaultReadFS struct {
	DefaultFileSystem
	size  uint64
	exist bool
}

func (fs *DefaultReadFS) GetAttr(name string, context *Context) (*Attr, Status) {
	if name == "" {
		return &Attr{Mode: S_IFDIR | 0755}, OK
	}
	if name == "file" {
		return &Attr{Mode: S_IFREG | 0644, Size: fs.size}, OK
	}
	return nil, ENOENT
}

func (fs *DefaultReadFS) Open(name string, f uint32, context *Context) (File, Status) {
	return &DefaultFile{}, OK
}


func defaultReadTest() (root string, cleanup func()) {
	fs := &NotifyFs{}
	var err error
	dir, err := ioutil.TempDir("", "go-fuse")
	CheckSuccess(err)
	pathfs := NewPathNodeFs(fs, nil)
	state, _, err := MountNodeFileSystem(dir, pathfs, nil)
	CheckSuccess(err)
	state.Debug = VerboseTest()
	go state.Loop()
	
	return dir, func() {
		state.Unmount()
		os.Remove(dir)
	}
}

func TestDefaultRead(t *testing.T) {
	root, clean := defaultReadTest()
	defer clean()

	_, err := ioutil.ReadFile(root + "/file")
	if err == nil {
		t.Fatal("should have failed read.")
	}
}
