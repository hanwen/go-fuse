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

	// If we are actually mounted, we should get ENOSYS
	var st syscall.Stat_t
	err = syscall.Stat(realMountPoint, &st)
	if err != syscall.ENOSYS {
		t.Errorf("expected ENOSYS, got %v", err)
	}

	// Cleanup is somewhat tricky because `srv` does not know about
	// `realMountPoint`, so `srv.Unmount()` cannot work.
	// We could call `fusermount -u` ourselves, but here we can also just
	// monkey-patch `srv.mountPoint`.
	srv.mountPoint = realMountPoint
	if err := srv.Unmount(); err != nil {
		t.Error(err)
	} else {
		srv.Wait()
	}
}
