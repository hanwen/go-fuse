// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/sys/unix"
)

func TestRenameExchange(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

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

	var before1, before2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &before1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &before2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}

	if err := unix.Renameat2(f1, "file", f2, "file", unix.RENAME_EXCHANGE); err != nil {
		t.Errorf("rename EXCHANGE: %v", err)
	}

	var after1, after2 unix.Stat_t
	if err := unix.Fstatat(f1, "file", &after1, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	if err := unix.Fstatat(f2, "file", &after2, 0); err != nil {
		t.Fatalf("Fstatat: %v", err)
	}
	clearCtime := func(s *unix.Stat_t) {
		s.Ctim.Sec = 0
		s.Ctim.Nsec = 0
	}

	clearCtime(&after1)
	clearCtime(&after2)
	clearCtime(&before2)
	clearCtime(&before1)
	if diff := pretty.Compare(after1, before2); diff != "" {
		t.Errorf("after1, before2: %s", diff)
	}
	if !reflect.DeepEqual(after2, before1) {
		t.Errorf("after2, before1: %#v, %#v", after2, before1)
	}
}

func TestRenameNoOverwrite(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

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
	defer tc.Clean()

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
	sz, err := syscall.Getxattr(tc.mntDir+"/file", attr, buf)
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

func TestCopyFileRange(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

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
