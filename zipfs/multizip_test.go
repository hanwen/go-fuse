package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

var _ = log.Printf
var CheckSuccess = fuse.CheckSuccess

const testTtl = 0.1

func setupMzfs() (mountPoint string, cleanup func()) {
	fs := NewMultiZipFs()
	mountPoint, _ = ioutil.TempDir("", "")
	state, _, err := fuse.MountPathFileSystem(mountPoint, fs, &fuse.FileSystemOptions{
		EntryTimeout:    testTtl,
		AttrTimeout:     testTtl,
		NegativeTimeout: 0.0,
	})
	CheckSuccess(err)
	state.Debug = fuse.VerboseTest()
	go state.Loop()

	return mountPoint, func() {
		state.Unmount()
		os.RemoveAll(mountPoint)
	}
}

func TestMultiZipReadonly(t *testing.T) {
	mountPoint, cleanup := setupMzfs()
	defer cleanup()

	_, err := os.Create(mountPoint + "/random")
	if err == nil {
		t.Error("Must fail writing in root.")
	}

	_, err = os.OpenFile(mountPoint+"/config/zipmount", os.O_WRONLY, 0)
	if err == nil {
		t.Error("Must fail without O_CREATE")
	}
}

func TestMultiZipFs(t *testing.T) {
	mountPoint, cleanup := setupMzfs()
	defer cleanup()

	zipFile := testZipFile()

	entries, err := ioutil.ReadDir(mountPoint)
	CheckSuccess(err)
	if len(entries) != 1 || string(entries[0].Name) != "config" {
		t.Errorf("wrong names return. %v", entries)
	}

	err = os.Symlink(zipFile, mountPoint+"/config/zipmount")
	CheckSuccess(err)

	fi, err := os.Lstat(mountPoint + "/zipmount")
	if !fi.IsDirectory() {
		t.Errorf("Expect directory at /zipmount")
	}

	entries, err = ioutil.ReadDir(mountPoint)
	CheckSuccess(err)
	if len(entries) != 2 {
		t.Error("Expect 2 entries", entries)
	}

	val, err := os.Readlink(mountPoint + "/config/zipmount")
	CheckSuccess(err)
	if val != zipFile {
		t.Errorf("expected %v got %v", zipFile, val)
	}

	fi, err = os.Lstat(mountPoint + "/zipmount")
	CheckSuccess(err)
	if !fi.IsDirectory() {
		t.Fatal("expect directory for /zipmount, got %v", fi)
	}

	// Check that zipfs itself works.
	fi, err = os.Stat(mountPoint + "/zipmount/subdir")
	CheckSuccess(err)
	if !fi.IsDirectory() {
		t.Error("directory type", fi)
	}

	// Removing the config dir unmount
	err = os.Remove(mountPoint + "/config/zipmount")
	CheckSuccess(err)

	fi, err = os.Stat(mountPoint + "/zipmount")
	if err == nil {
		t.Error("stat should fail after unmount.", fi)
	}
}
