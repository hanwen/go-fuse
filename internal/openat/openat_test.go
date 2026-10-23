// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openat

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenat(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "f"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	for _, l := range [][2]string{{"a", "l"}, {"a/f", "fl"}, {"/", "abs"}} {
		if err := os.Symlink(l[0], filepath.Join(dir, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	dirfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(dirfd)

	for name, open := range map[string]func(int, string, int, uint32) (int, error){
		"Openat": Openat,
		"walk":   walk,
	} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				path string
				ok   bool
			}{
				{".", true},
				{"a", true},
				{"a/f", true},
				{"a/b", true},
				{"./a//f", true},
				{"l/f", false},
				{"fl", false},
				{"abs/etc", false},
			} {
				fd, err := open(dirfd, tc.path, unix.O_RDONLY, 0)
				if err == nil {
					unix.Close(fd)
				}
				if (err == nil) != tc.ok {
					t.Errorf("%q: got err %v, want ok=%v", tc.path, err, tc.ok)
				}
			}
		})
	}

	for _, p := range []string{"", "/etc", "..", "a/../a"} {
		if fd, err := Openat(dirfd, p, unix.O_RDONLY, 0); err == nil {
			unix.Close(fd)
			t.Errorf("%q: opened", p)
		}
	}
}
