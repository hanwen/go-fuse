package fuse

import (
	"io/ioutil"
	"os"
	"syscall"
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

	var stat syscall.Stat_t
	err := syscall.Lstat(wd+"/foo", &stat)
	CheckSuccess(err)

	if int(stat.Uid) != os.Getuid() || int(stat.Gid) != os.Getgid() {
		t.Fatal("Should use current uid for mount")
	}
}

func TestOwnerRoot(t *testing.T) {
	wd, cleanup := setupOwnerTest(&FileSystemOptions{})
	defer cleanup()

	var st syscall.Stat_t
	err := syscall.Lstat(wd+"/foo", &st)
	CheckSuccess(err)

	if st.Uid != _RANDOM_OWNER || st.Gid != _RANDOM_OWNER {
		t.Fatal("Should use FS owner uid")
	}
}

func TestOwnerOverride(t *testing.T) {
	wd, cleanup := setupOwnerTest(&FileSystemOptions{Owner: &Owner{42, 43}})
	defer cleanup()

	var stat syscall.Stat_t
	err := syscall.Lstat(wd+"/foo", &stat)
	CheckSuccess(err)

	if stat.Uid != 42 || stat.Gid != 43 {
		t.Fatal("Should use current uid for mount")
	}
}
