package fuse

import (
	"os"
	"testing"
)


type ownerFs struct {
	DefaultFileSystem
}

const _RANDOM_OWNER = 31415265

func (me *ownerFs) GetAttr(name string) (*os.FileInfo, Status) {
	if name == "" {
		return &os.FileInfo{
			Mode: S_IFDIR | 0755,
		}, OK
	}
	return &os.FileInfo{
		Mode: S_IFREG | 0644,
		Uid: _RANDOM_OWNER,
		Gid: _RANDOM_OWNER,
	}, OK
}

func setupOwnerTest(opts *FileSystemOptions) (workdir string, cleanup func()) {
	wd := MakeTempDir()

	fs := &ownerFs{}
	state, _, err := MountFileSystem(wd, fs, opts)
	CheckSuccess(err)
	go state.Loop(false)
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

