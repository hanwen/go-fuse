// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"io/ioutil"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sys/unix"
)

func TestRenameNoOverwrite(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})

	if err := os.Mkdir(tc.origDir+"/dir", 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	tc.writeOrig("file", "hello", 0644)
	tc.writeOrig("dir/file", "x", 0644)

	f1, err := syscall.Open(tc.mntDir+"/", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	defer syscall.Close(f1)
	f2, err := syscall.Open(tc.mntDir+"/dir", syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer syscall.Close(f2)

	if err := unix.Renameat2(f1, "file", f2, "file", unix.RENAME_NOREPLACE); err == nil {
		t.Errorf("rename NOREPLACE succeeded")
	} else if err != syscall.EEXIST {
		t.Errorf("got %v (%T) want EEXIST", err, err)
	}
}

func TestXAttr(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})

	tc.writeOrig("file", "", 0644)

	buf := make([]byte, 1024)
	attr := "user.xattrtest"
	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err == syscall.ENOTSUP {
		t.Skip("$TMP does not support xattrs. Rerun this test with a $TMPDIR override")
	}

	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err != syscall.ENODATA {
		t.Fatalf("got %v want ENOATTR", err)
	}
	value := []byte("value")
	if err := syscall.Setxattr(tc.mntDir+"/file", attr, value, 0); err != nil {
		t.Fatalf("Setxattr: %v", err)
	}

	sz, err := syscall.Listxattr(tc.mntDir+"/file", nil)
	if err != nil {
		t.Fatalf("Listxattr: %v", err)
	}
	buf = make([]byte, sz)
	if _, err := syscall.Listxattr(tc.mntDir+"/file", buf); err != nil {
		t.Fatalf("Listxattr: %v", err)
	} else {
		attributes := bytes.Split(buf[:sz], []byte{0})
		found := false
		for _, a := range attributes {
			if string(a) == attr {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Listxattr: %q (not found: %q", buf[:sz], attr)
		}
	}

	sz, err = syscall.Getxattr(tc.mntDir+"/file", attr, buf)
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	}
	if bytes.Compare(buf[:sz], value) != 0 {
		t.Fatalf("Getxattr got %q want %q", buf[:sz], value)
	}
	if err := syscall.Removexattr(tc.mntDir+"/file", attr); err != nil {
		t.Fatalf("Removexattr: %v", err)
	}

	if _, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf); err != syscall.ENODATA {
		t.Fatalf("got %v want ENOATTR", err)
	}
}

// TestXAttrSymlink verifies that we did not forget to use Lgetxattr instead
// of Getxattr. This test is Linux-specific because it depends on the behavoir
// of the `security` namespace.
//
// On Linux, symlinks can not have xattrs in the `user` namespace, so we
// try to read something from `security`. Writing would need root rights,
// so don't even bother. See `man 7 xattr` for more info.
func TestXAttrSymlink(t *testing.T) {
	tc := newTestCase(t, nil)

	path := tc.mntDir + "/symlink"
	if err := syscall.Symlink("target/does/not/exist", path); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 10)
	_, err := unix.Lgetxattr(path, "security.foo", buf)
	if err != unix.ENODATA {
		t.Errorf("want %d=ENODATA, got error %d=%q instead", unix.ENODATA, err, err)
	}
}

func TestCopyFileRange(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})

	if !tc.server.KernelSettings().SupportsVersion(7, 28) {
		t.Skip("need v7.28 for CopyFileRange")
	}

	tc.writeOrig("src", "01234567890123456789", 0644)
	tc.writeOrig("dst", "abcdefghijabcdefghij", 0644)

	f1, err := syscall.Open(tc.mntDir+"/src", syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Open src: %v", err)
	}
	defer func() {
		// syscall.Close() is treacherous; because fds are
		// reused, a double close can cause serious havoc
		if f1 > 0 {
			syscall.Close(f1)
		}
	}()

	f2, err := syscall.Open(tc.mntDir+"/dst", syscall.O_RDWR, 0)
	if err != nil {
		t.Fatalf("Open dst: %v", err)
	}
	defer func() {
		if f2 > 0 {
			defer syscall.Close(f2)
		}
	}()

	srcOff := int64(5)
	dstOff := int64(7)
	if sz, err := unix.CopyFileRange(f1, &srcOff, f2, &dstOff, 3, 0); err != nil || sz != 3 {
		t.Fatalf("CopyFileRange: %d,%v", sz, err)
	}

	err = syscall.Close(f1)
	f1 = 0
	if err != nil {
		t.Fatalf("Close src: %v", err)
	}

	err = syscall.Close(f2)
	f2 = 0
	if err != nil {
		t.Fatalf("Close dst: %v", err)
	}
	c, err := ioutil.ReadFile(tc.mntDir + "/dst")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	want := "abcdefg567abcdefghij"
	got := string(c)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}

}

// Wait for a change in /proc/self/mounts. Efficient through the use of
// unix.Poll().
func waitProcMountsChange() error {
	fd, err := syscall.Open("/proc/self/mounts", syscall.O_RDONLY, 0)
	defer syscall.Close(fd)
	if err != nil {
		return err
	}
	pollFds := []unix.PollFd{
		{
			Fd:     int32(fd),
			Events: unix.POLLPRI,
		},
	}
	_, err = unix.Poll(pollFds, 1000)
	return err
}

// Wait until mountpoint "mnt" shows up /proc/self/mounts
func waitMount(mnt string) error {
	for {
		err := waitProcMountsChange()
		if err != nil {
			return err
		}
		content, err := ioutil.ReadFile("/proc/self/mounts")
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte(mnt)) {
			return nil
		}
	}
}

// There is a hang that appears when enabling CAP_PARALLEL_DIROPS on Linux
// 4.15.0: https://github.com/hanwen/go-fuse/issues/281
// The hang was originally triggered by gvfs-udisks2-volume-monitor. This
// test emulates what gvfs-udisks2-volume-monitor does.
func TestParallelDiropsHang(t *testing.T) {
	// We do NOT want to use newTestCase() here because we need to know the
	// mnt path before the filesystem is mounted
	dir := t.TempDir()
	orig := dir + "/orig"
	mnt := dir + "/mnt"
	if err := os.Mkdir(orig, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mnt, 0755); err != nil {
		t.Fatal(err)
	}

	// Unblock the goroutines onces the mount shows up in /proc/self/mounts
	wait := make(chan struct{})
	go func() {
		err := waitMount(mnt)
		if err != nil {
			t.Error(err)
		}
		// Unblock the goroutines regardless of an error. We don't want to hang
		// the test.
		close(wait)
	}()

	// gvfs-udisks2-volume-monitor hits the mount with three threads - we try to
	// emulate exactly what it does acc. to an strace log.
	var wg sync.WaitGroup
	wg.Add(3)
	// [pid  2117] lstat(".../mnt/autorun.inf",  <unfinished ...>
	go func() {
		defer wg.Done()
		<-wait
		var st unix.Stat_t
		unix.Lstat(mnt+"/autorun.inf", &st)
	}()
	// [pid  2116] open(".../mnt/.xdg-volume-info", O_RDONLY <unfinished ...>
	go func() {
		defer wg.Done()
		<-wait
		syscall.Open(mnt+"/.xdg-volume-info", syscall.O_RDONLY, 0)
	}()
	// 25 times this:
	// [pid  1874] open(".../mnt", O_RDONLY|O_NONBLOCK|O_DIRECTORY|O_CLOEXEC <unfinished ...>
	// [pid  1874] fstat(11, {st_mode=S_IFDIR|0775, st_size=4096, ...}) = 0
	// [pid  1874] getdents(11, /* 2 entries */, 32768) = 48
	// [pid  1874] close(11)                   = 0
	go func() {
		defer wg.Done()
		<-wait
		for i := 1; i <= 25; i++ {
			f, err := os.Open(mnt)
			if err != nil {
				t.Error(err)
				return
			}
			_, err = f.Stat()
			if err != nil {
				t.Error(err)
				f.Close()
				return
			}
			_, err = f.Readdirnames(-1)
			if err != nil {
				t.Errorf("iteration %d: fd %d: %v", i, f.Fd(), err)
				return
			}
			f.Close()
		}
	}()

	loopbackRoot, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatalf("NewLoopbackRoot(%s): %v\n", orig, err)
	}
	sec := time.Second
	opts := &Options{
		AttrTimeout:  &sec,
		EntryTimeout: &sec,
	}
	opts.Debug = testutil.VerboseTest()

	rawFS := NewNodeFS(loopbackRoot, opts)
	server, err := fuse.NewServer(rawFS, mnt, &opts.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()

	wg.Wait()
	server.Unmount()
}

func TestRoMount(t *testing.T) {
	newTestCase(t, &testOptions{ro: true})
}

func TestDirectMount(t *testing.T) {
	opts := &testOptions{
		directMount: true,
	}
	if os.Geteuid() == 0 {
		t.Log("running as root, setting DirectMountStrict")
		opts.directMountStrict = true
	}
	newTestCase(t, opts)
}
