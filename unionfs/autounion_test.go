package unionfs

import (
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"fmt"
	"log"
	"testing"
	"time"
)

var _ = fmt.Print
var _ = log.Print

const entryTtl = 0.1

var testAOpts = AutoUnionFsOptions{
	UnionFsOptions: testOpts,
	MountOptions: fuse.MountOptions{
		EntryTimeout:    entryTtl,
		AttrTimeout:     entryTtl,
		NegativeTimeout: 0,
	},
}


func WriteFile(name string, contents string) {
	err := ioutil.WriteFile(name, []byte(contents), 0644)
	CheckSuccess(err)
}

func setup(t *testing.T) (workdir string, cleanup func()) {
	wd := fuse.MakeTempDir()
	err := os.Mkdir(wd+"/mount", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/store", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)
	WriteFile(wd+"/ro/file1", "file1")
	WriteFile(wd+"/ro/file2", "file2")

	fs := NewAutoUnionFs(wd+"/store", testAOpts)
	connector := fuse.NewFileSystemConnector(fs, &testAOpts.MountOptions)
	state := fuse.NewMountState(connector)
	state.Mount(wd + "/mount")
	state.Debug = true
	go state.Loop(false)

	return wd, func() {
		state.Unmount()
		os.RemoveAll(wd)
	}
}

func TestAutoFsSymlink(t *testing.T) {
	wd, clean := setup(t)
	defer clean()
	
	err := os.Mkdir(wd+"/store/foo", 0755)
	CheckSuccess(err)
	os.Symlink(wd+"/ro", wd+"/store/foo/READONLY")
	CheckSuccess(err)

	err = os.Symlink(wd+"/store/foo", wd+"/mount/config/bar")
	CheckSuccess(err)

	fi, err := os.Lstat(wd+"/mount/bar/file1")
	CheckSuccess(err)

	err = os.Remove(wd+"/mount/config/bar")
	CheckSuccess(err)

	// Need time for the unmount to be noticed.
	log.Println("sleeping...")
	time.Sleep(entryTtl*2e9)

	fi, _ = os.Lstat(wd+"/mount/foo")
	if fi != nil {
		t.Error("Should not have file:", fi)
	}

	_, err = ioutil.ReadDir(wd+"/mount/config")
	CheckSuccess(err)

	_, err = os.Lstat(wd+"/mount/foo/file1")
	CheckSuccess(err)
}

func TestCreationChecks(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	err := os.Mkdir(wd+"/store/foo", 0755)
	CheckSuccess(err)
	os.Symlink(wd+"/ro", wd+"/store/foo/READONLY")
	CheckSuccess(err)

	err = os.Mkdir(wd+"/store/ws2", 0755)
	CheckSuccess(err)
	os.Symlink(wd+"/ro", wd+"/store/ws2/READONLY")
	CheckSuccess(err)

	err = os.Symlink(wd+"/store/foo", wd+"/mount/config/bar")
	CheckSuccess(err)

	err = os.Symlink(wd+"/store/foo", wd+"/mount/config/foo")
	code := fuse.OsErrorToErrno(err)
	if code != fuse.EBUSY {
		t.Error("Should return EBUSY", err)
	}

	err = os.Symlink(wd+"/store/ws2", wd+"/mount/config/config")
	code = fuse.OsErrorToErrno(err)
	if code != fuse.EINVAL {
		t.Error("Should return EINVAL", err)
	}

}
