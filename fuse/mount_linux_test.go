package fuse

import (
	"fmt"
	"io/ioutil"
	"syscall"
	"testing"
)

// TestMountDevFd tests the special `/dev/fd/N` mountpoint syntax, where a
// privileged parent process opens /dev/fuse and calls mount() for us.
//
// In this test, we simulate a privileged parent by using the `fusermount` suid
// helper.
func TestMountDevFd(t *testing.T) {
	realMountPoint, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Rmdir(realMountPoint)

	// Call the fusermount suid helper to obtain the file descriptor in place
	// of a privileged parent.
	var fuOpts MountOptions
	fd, err := callFusermount(realMountPoint, &fuOpts)
	if err != nil {
		t.Fatal(err)
	}
	fdMountPoint := fmt.Sprintf("/dev/fd/%d", fd)

	// Real test starts here:
	// See if we can feed fdMountPoint to NewServer
	fs := NewDefaultRawFileSystem()
	opts := MountOptions{
		Debug: true,
	}
	srv, err := NewServer(fs, fdMountPoint, &opts)
	if err != nil {
		t.Fatal(err)
	}

	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}

	// If we are actually mounted, we should get ENOSYS.
	//
	// This won't deadlock despite pollHack not working for `/dev/fd/N` mounts
	// because functions in the syscall package don't use the poller.
	var st syscall.Stat_t
	err = syscall.Stat(realMountPoint, &st)
	if err != syscall.ENOSYS {
		t.Errorf("expected ENOSYS, got %v", err)
	}

	// Cleanup is somewhat tricky because `srv` does not know about
	// `realMountPoint`, so `srv.Unmount()` cannot work.
	//
	// A normal user has to call `fusermount -u` for themselves to unmount.
	// But in this test we can monkey-patch `srv.mountPoint`.
	srv.mountPoint = realMountPoint
	if err := srv.Unmount(); err != nil {
		t.Error(err)
	}
}

// TestMountMaxWrite makes sure that mounting works with all MaxWrite settings.
// We used to fail with EINVAL below 8k because readPool got too small.
func TestMountMaxWrite(t *testing.T) {
	opts := []MountOptions{
		{MaxWrite: 0}, // go-fuse default
		{MaxWrite: 1},
		{MaxWrite: 123},
		{MaxWrite: 1 * 1024},
		{MaxWrite: 4 * 1024},
		{MaxWrite: 8 * 1024},
		{MaxWrite: 64 * 1024},  // go-fuse default
		{MaxWrite: 128 * 1024}, // limit in Linux v4.19 and older
		{MaxWrite: 999 * 1024},
		{MaxWrite: 1024 * 1024}, // limit in Linux v4.20+
	}
	for _, o := range opts {
		name := fmt.Sprintf("MaxWrite%d", o.MaxWrite)
		t.Run(name, func(t *testing.T) {
			mnt, err := ioutil.TempDir("", name)
			if err != nil {
				t.Fatal(err)
			}
			fs := NewDefaultRawFileSystem()
			srv, err := NewServer(fs, mnt, &o)
			if err != nil {
				t.Error(err)
			} else {
				go srv.Serve()
				srv.Unmount()
			}
		})
	}
}
