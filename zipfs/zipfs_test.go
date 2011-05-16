package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"testing"
)

func TestZipFs(t *testing.T) {
	wd, err := os.Getwd()
	CheckSuccess(err)
	zfs, err := NewArchiveFileSystem(wd + "/test.zip")
	if err != nil {
		t.Error("NewZipArchiveFileSystem failed:", err)
	}

	mountPoint := fuse.MakeTempDir()
	defer os.RemoveAll(mountPoint)
	state, _, err := fuse.MountFileSystem(mountPoint, zfs, nil)
	defer state.Unmount()

	state.Debug = true
	go state.Loop(false)

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
