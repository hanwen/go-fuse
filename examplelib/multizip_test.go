package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"testing"
	"time"
)

func TestMultiZipFs(t *testing.T) {
	var err os.Error

	wd, err := os.Getwd()
	zipFile := wd + "/test.zip"

	fs := NewMultiZipFs()
	state := fuse.NewMountState(fs.Connector)
	mountPoint := fuse.MakeTempDir()

	state.Debug = true
	err = state.Mount(mountPoint)
	CheckSuccess(err)

	go state.Loop(true)

	f, err := os.Open(mountPoint+"", os.O_RDONLY, 0)
	CheckSuccess(err)

	names, err := f.Readdirnames(-1)
	CheckSuccess(err)

	if len(names) != 1 || string(names[0]) != "config" {
		t.Errorf("wrong names return. %v", names)
	}
	err = f.Close()
	CheckSuccess(err)

	f, err = os.Open(mountPoint+"/random", os.O_WRONLY|os.O_CREATE, 0)
	if err == nil {
		t.Error("Must fail writing in root.")
	}

	f, err = os.Open(mountPoint+"/config/zipmount", os.O_WRONLY, 0)
	if err == nil {
		t.Error("Must fail without O_CREATE")
	}
	f, err = os.Open(mountPoint+"/config/zipmount", os.O_WRONLY|os.O_CREATE, 0)
	CheckSuccess(err)

	// Directory exists, but is empty.
	if !IsDir(mountPoint + "/zipmount") {
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

	if !IsDir(mountPoint + "/zipmount") {
		t.Errorf("Expect directory at /zipmount")
	}

	// Check that zipfs itself works.
	fi, err := os.Stat(mountPoint + "/zipmount/subdir")
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

	state.Unmount()
}
