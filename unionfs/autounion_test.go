package unionfs

import (
	"os"
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"fmt"
	"log"
	"testing"
)

var _ = fmt.Print
var _ = log.Print

const entryTtl = 0.1

var testAOpts = AutoUnionFsOptions{
	UnionFsOptions: testOpts,
	FileSystemOptions: fuse.FileSystemOptions{
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
	wd, _ := ioutil.TempDir("", "")
	err := os.Mkdir(wd+"/mnt", 0700)
	fuse.CheckSuccess(err)

	err = os.Mkdir(wd+"/store", 0700)
	fuse.CheckSuccess(err)

	os.Mkdir(wd+"/ro", 0700)
	fuse.CheckSuccess(err)
	WriteFile(wd+"/ro/file1", "file1")
	WriteFile(wd+"/ro/file2", "file2")

	fs := NewAutoUnionFs(wd+"/store", testAOpts)
	state, conn, err := fuse.MountPathFileSystem(wd+"/mnt", fs, &testAOpts.FileSystemOptions)
	CheckSuccess(err)
	state.Debug = fuse.VerboseTest()
	conn.Debug = fuse.VerboseTest()
	go state.Loop()

	return wd, func() {
		state.Unmount()
		os.RemoveAll(wd)
	}
}

func TestVersion(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	c, err := ioutil.ReadFile(wd + "/mnt/status/gounionfs_version")
	CheckSuccess(err)
	if len(c) == 0 {
		t.Fatal("No version found.")
	}
	log.Println("Found version:", string(c))
}

func TestAutoFsSymlink(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	err := os.Mkdir(wd+"/store/backing1", 0755)
	CheckSuccess(err)

	err = os.Symlink(wd+"/ro", wd+"/store/backing1/READONLY")
	CheckSuccess(err)

	err = os.Symlink(wd+"/store/backing1", wd+"/mnt/config/manual1")
	CheckSuccess(err)

	fi, err := os.Lstat(wd + "/mnt/manual1/file1")
	CheckSuccess(err)

	entries, err := ioutil.ReadDir(wd + "/mnt")
	CheckSuccess(err)
	if len(entries) != 3 {
		t.Error("readdir mismatch", entries)
	}

	err = os.Remove(wd + "/mnt/config/manual1")
	CheckSuccess(err)

	scan := wd + "/mnt/config/" + _SCAN_CONFIG
	err = ioutil.WriteFile(scan, []byte("something"), 0644)
	if err != nil {
		t.Error("error writing:", err)
	}

	fi, _ = os.Lstat(wd + "/mnt/manual1")
	if fi != nil {
		t.Error("Should not have file:", fi)
	}

	_, err = ioutil.ReadDir(wd + "/mnt/config")
	CheckSuccess(err)

	_, err = os.Lstat(wd + "/mnt/backing1/file1")
	CheckSuccess(err)
}

func TestDetectSymlinkedDirectories(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	err := os.Mkdir(wd+"/backing1", 0755)
	CheckSuccess(err)

	err = os.Symlink(wd+"/ro", wd+"/backing1/READONLY")
	CheckSuccess(err)

	err = os.Symlink(wd+"/backing1", wd+"/store/backing1")
	CheckSuccess(err)

	scan := wd + "/mnt/config/" + _SCAN_CONFIG
	err = ioutil.WriteFile(scan, []byte("something"), 0644)
	if err != nil {
		t.Error("error writing:", err)
	}

	_, err = os.Lstat(wd + "/mnt/backing1")
	CheckSuccess(err)
}

func TestExplicitScan(t *testing.T) {
	wd, clean := setup(t)
	defer clean()

	err := os.Mkdir(wd+"/store/backing1", 0755)
	CheckSuccess(err)
	os.Symlink(wd+"/ro", wd+"/store/backing1/READONLY")
	CheckSuccess(err)

	fi, _ := os.Lstat(wd + "/mnt/backing1")
	if fi != nil {
		t.Error("Should not have file:", fi)
	}

	scan := wd + "/mnt/config/" + _SCAN_CONFIG
	_, err = os.Lstat(scan)
	if err != nil {
		t.Error(".scan_config missing:", err)
	}

	err = ioutil.WriteFile(scan, []byte("something"), 0644)
	if err != nil {
		t.Error("error writing:", err)
	}

	_, err = os.Lstat(wd + "/mnt/backing1")
	if err != nil {
		t.Error("Should have workspace backing1:", err)
	}
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

	err = os.Symlink(wd+"/store/foo", wd+"/mnt/config/bar")
	CheckSuccess(err)

	err = os.Symlink(wd+"/store/foo", wd+"/mnt/config/foo")
	code := fuse.OsErrorToErrno(err)
	if code != fuse.EBUSY {
		t.Error("Should return EBUSY", err)
	}

	err = os.Symlink(wd+"/store/ws2", wd+"/mnt/config/config")
	code = fuse.OsErrorToErrno(err)
	if code != fuse.EINVAL {
		t.Error("Should return EINVAL", err)
	}
}
