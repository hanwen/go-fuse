package fuse

import (
	"io/ioutil"
	"os"
	"testing"
)

type ownerFs struct {
	DefaultFileSystem
}

const _RANDOM_OWNER = 31415265

func (me *ownerFs) GetAttr(name string, context *Context) (*Attr, Status) {
	if name == "" {
		return &Attr{
			Mode: S_IFDIR | 0755,
		}, OK
	}
	a := &Attr{
		Mode: S_IFREG | 0644,
	}
	a.Uid = _RANDOM_OWNER
	a.Gid = _RANDOM_OWNER
	return a, OK
}

func setupOwnerTest(opts *FileSystemOptions) (workdir string, cleanup func()) {
	wd, err := ioutil.TempDir("", "go-fuse")

	fs := &ownerFs{}
	nfs := NewPathNodeFs(fs, nil)
	state, _, err := MountNodeFileSystem(wd, nfs, opts)
	CheckSuccess(err)
	go state.Loop()
	return wd, func() {
		state.Unmount()
		os.RemoveAll(wd)
	}
}

func TestOwnerDefault(t *testing.T) {
	wd, cleanup := setupOwnerTest(NewFileSystemOptions())
	defer cleanup()
	fi, err := os.Lstat(wd + "/foo")
	CheckSuccess(err)

	if fi.Uid != os.Getuid() || fi.Gid != os.Getgid() {
		t.Fatal("Should use current uid for mount", fi.Uid, fi.Gid)
	}
}

func TestOwnerRoot(t *testing.T) {
	wd, cleanup := setupOwnerTest(&FileSystemOptions{})
	defer cleanup()
	fi, err := os.Lstat(wd + "/foo")
	CheckSuccess(err)

	if fi.Uid != _RANDOM_OWNER || fi.Gid != _RANDOM_OWNER {
		t.Fatal("Should use FS owner uid", fi.Uid, fi.Gid)
	}
}

func TestOwnerOverride(t *testing.T) {
	wd, cleanup := setupOwnerTest(&FileSystemOptions{Owner: &Owner{42, 43}})
	defer cleanup()
	fi, err := os.Lstat(wd + "/foo")
	CheckSuccess(err)

	if fi.Uid != 42 || fi.Gid != 43 {
		t.Fatal("Should use current uid for mount", fi.Uid, fi.Gid)
	}
}
