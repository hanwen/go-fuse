// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAtHelpers(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "native"
		if fallback {
			name = "proc"
		}
		t.Run(name, func(t *testing.T) {
			oldF, oldX := noFchmodat2.Load(), noXattrat.Load()
			defer func() {
				noFchmodat2.Store(oldF)
				noXattrat.Store(oldX)
			}()
			if fallback {
				noFchmodat2.Store(true)
				noXattrat.Store(true)
			}
			testAtHelpers(t)
			if !fallback && (noFchmodat2.Load() || noXattrat.Load()) {
				t.Log("kernel lacks fchmodat2 or xattrat; only the fallback was tested")
			}
		})
	}
}

func testAtHelpers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/file", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", dir+"/link"); err != nil {
		t.Fatal(err)
	}
	dirfd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dirfd)

	if err := fchmodatNoFollow(dirfd, "file", 0600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	var st syscall.Stat_t
	if err := syscall.Stat(dir+"/file", &st); err != nil {
		t.Fatal(err)
	}
	if st.Mode&07777 != 0600 {
		t.Errorf("mode %o, want 0600", st.Mode&07777)
	}
	if err := fchmodatNoFollow(dirfd, "link", 0644); err == nil {
		t.Errorf("chmod on symlink succeeded")
	}
	if err := syscall.Stat(dir+"/file", &st); err != nil {
		t.Fatal(err)
	}
	if st.Mode&07777 != 0600 {
		t.Errorf("chmod followed symlink: mode %o", st.Mode&07777)
	}

	val := []byte("value")
	if err := setxattrAt(dirfd, "file", "user.test", val, 0); err != nil {
		if err == unix.EOPNOTSUPP {
			t.Skip("no user xattrs on", dir)
		}
		t.Fatalf("setxattr: %v", err)
	}
	if sz, err := getxattrAt(dirfd, "file", "user.test", nil); err != nil || sz != len(val) {
		t.Errorf("getxattr size: %d, %v", sz, err)
	}
	buf := make([]byte, 64)
	sz, err := getxattrAt(dirfd, "file", "user.test", buf)
	if err != nil || !bytes.Equal(buf[:sz], val) {
		t.Errorf("getxattr: %q, %v", buf[:sz], err)
	}
	sz, err = listxattrAt(dirfd, "file", buf)
	if err != nil || !bytes.Contains(buf[:sz], []byte("user.test\x00")) {
		t.Errorf("listxattr: %q, %v", buf[:sz], err)
	}
	if _, err := getxattrAt(dirfd, "link", "user.test", buf); err == nil {
		t.Errorf("getxattr followed symlink")
	}
	if err := removexattrAt(dirfd, "file", "user.test"); err != nil {
		t.Errorf("removexattr: %v", err)
	}
	if _, err := getxattrAt(dirfd, "file", "user.test", buf); err != unix.ENODATA {
		t.Errorf("getxattr after remove: %v, want ENODATA", err)
	}
}
