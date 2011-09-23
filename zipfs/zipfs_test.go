package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testZipFile() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("need runtime.Caller()'s file name to discover testdata")
	}
	dir, _ := filepath.Split(file)
	return filepath.Join(dir, "test.zip")
}

func setupZipfs() (mountPoint string, cleanup func()) {
	zfs, err := NewArchiveFileSystem(testZipFile())
	CheckSuccess(err)

	mountPoint, _ = ioutil.TempDir("", "")
	state, _, err := fuse.MountNodeFileSystem(mountPoint, zfs, nil)

	state.Debug = fuse.VerboseTest()
	go state.Loop()

	return mountPoint, func() {
		state.Unmount()
		os.RemoveAll(mountPoint)
	}
}

func TestZipFs(t *testing.T) {
	mountPoint, clean := setupZipfs()
	defer clean()
	entries, err := ioutil.ReadDir(mountPoint)
	CheckSuccess(err)

	if len(entries) != 2 {
		t.Error("wrong length", entries)
	}
	fi, err := os.Stat(mountPoint + "/subdir")
	CheckSuccess(err)
	if !fi.IsDirectory() {
		t.Error("directory type", fi)
	}

	fi, err = os.Stat(mountPoint + "/file.txt")
	CheckSuccess(err)

	if !fi.IsRegular() {
		t.Error("file type", fi)
	}

	f, err := os.Open(mountPoint + "/file.txt")
	CheckSuccess(err)

	b := make([]byte, 1024)
	n, err := f.Read(b)

	b = b[:n]
	if string(b) != "hello\n" {
		t.Error("content fail", b[:n])
	}
	f.Close()
}

func TestLinkCount(t *testing.T) {
	mp, clean := setupZipfs()
	defer clean()

	fi, err := os.Stat(mp + "/file.txt")
	CheckSuccess(err)
	if fi.Nlink != 1 {
		t.Fatal("wrong link count", fi.Nlink)
	}
}
