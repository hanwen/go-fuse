package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"log"
	"os"
	"testing"
	"time"
)

var _ = log.Printf
var CheckSuccess = fuse.CheckSuccess

func TestMultiZipFs(t *testing.T) {
	var err os.Error

	wd, err := os.Getwd()
	zipFile := wd + "/test.zip"

	fs := NewMultiZipFs()
	mountPoint := fuse.MakeTempDir()
	state, _, err := fuse.MountFileSystem(mountPoint, fs, nil)
	defer os.RemoveAll(mountPoint)
	CheckSuccess(err)
	defer state.Unmount()
	
	state.Debug = true

	go state.Loop(true)

	f, err := os.Open(mountPoint + "")
	CheckSuccess(err)

	names, err := f.Readdirnames(-1)
	CheckSuccess(err)

	if len(names) != 1 || string(names[0]) != "config" {
		t.Errorf("wrong names return. %v", names)
	}
	err = f.Close()
	CheckSuccess(err)

	f, err = os.Create(mountPoint + "/random")
	if err == nil {
		t.Error("Must fail writing in root.")
	}

	f, err = os.OpenFile(mountPoint+"/config/zipmount", os.O_WRONLY, 0)
	if err == nil {
		t.Error("Must fail without O_CREATE")
	}
	f, err = os.Create(mountPoint + "/config/zipmount")
	CheckSuccess(err)

	// Directory exists, but is empty.
	fi, err := os.Lstat(mountPoint + "/zipmount")
	CheckSuccess(err)
	if !fi.IsDirectory() {
		t.Errorf("Expect directory at /zipmount")
	}

	// Open the zip file.
	_, err = f.Write([]byte(zipFile))
	CheckSuccess(err)

	_, err = f.Write([]byte(zipFile))
	if err == nil {
		t.Error("Must fail second write.")
	}

	err = f.Close()
	CheckSuccess(err)
	fi, err = os.Lstat(mountPoint + "/zipmount")
	if !fi.IsDirectory() {
		t.Errorf("Expect directory at /zipmount")
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

	// This is ugly but necessary: We don't have ways to signal
	// back to FUSE that the file disappeared.
	time.Sleep(1.5e9)

	fi, err = os.Stat(mountPoint + "/zipmount")
	if err == nil {
		t.Error("stat should fail after unmount.", fi)
	}
}
