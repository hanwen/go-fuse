
package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"testing"
)

func setupZipfs() (mountPoint string, cleanup func()) {
	wd, err := os.Getwd()
	CheckSuccess(err)
	zfs, err := NewArchiveFileSystem(wd + "/test.zip")
	CheckSuccess(err) 

	mountPoint = fuse.MakeTempDir()

	state, _, err := fuse.MountFileSystem(mountPoint, zfs, nil)
	
	state.Debug = true
	go state.Loop(false)

	return mountPoint, func() {
		state.Unmount()
		os.RemoveAll(mountPoint)
	}
}

func TestZipFs(t *testing.T) {
	mountPoint, clean := setupZipfs()
	defer clean()

	d, err := os.Open(mountPoint)
	CheckSuccess(err)

	names, err := d.Readdirnames(-1)
	CheckSuccess(err)
	err = d.Close()
	CheckSuccess(err)

	if len(names) != 2 {
		t.Error("wrong length", names)
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
	t.Log("TestLinkCount")
	mp, clean := setupZipfs()
	defer clean()

	fi, err := os.Stat(mp + "/file.txt")
	CheckSuccess(err)
	if fi.Nlink != 1  {
		t.Fatal("wrong link count", fi.Nlink)
	}
}

