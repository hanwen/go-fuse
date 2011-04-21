package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"testing"
)

func TestZipFs(t *testing.T) {
	wd, err := os.Getwd()
	CheckSuccess(err)
	zfs := NewZipArchiveFileSystem(wd + "/test.zip")

	connector := fuse.NewFileSystemConnector(zfs)
	mountPoint := fuse.MakeTempDir()

	state := fuse.NewMountState(connector)
	state.Mount(mountPoint)

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
	if string(b[:n]) != "hello\n" {
		t.Error("content fail", b[:n])
	}
	f.Close()

	state.Unmount()
}
