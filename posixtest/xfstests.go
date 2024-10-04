// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package posixtest

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

// DirSeek tests that seeking on a directory works for
// https://github.com/hanwen/go-fuse/issues/344 .
//
// Go port of xfstests generic/257.
//
// Hint:
// $ go test ./fs -run TestPosix/DirSeek -v
func DirSeek(t *testing.T, mnt string) {
	// From bash script xfstests/tests/generic/257
	ttt := mnt + "/ttt"
	err := os.Mkdir(ttt, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 168; i++ {
		path := fmt.Sprintf("%s/%d", ttt, i)
		err = os.WriteFile(path, nil, 0600)
		if err != nil {
			t.Fatal(err)
		}
	}

	// From C app xfstests/src/t_dir_offset2.c
	const bufSize = 4096
	buf := make([]byte, bufSize)
	fd, err := syscall.Open(ttt, syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	result, err := readAllDirEntries(fd)
	if err != nil {
		t.Fatal(err)
	}

	// check if seek works correctly
	for i := len(result) - 1; i >= 0; i-- {
		var seekTo int64
		if i > 0 {
			seekTo = int64(result[i-1].Off)
		}
		_, err = unix.Seek(fd, seekTo, os.SEEK_SET)
		if err != nil {
			t.Fatal(err)
		}

		n, err := unix.ReadDirent(fd, buf)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("getdents returned 0 on entry %d", i)
			continue
		}

		var de fuse.DirEntry
		de.Parse(buf)

		if !reflect.DeepEqual(&de, &result[i]) {
			t.Errorf("entry %d: got %v, want %v",
				i, de, result[i])
		}
	}
}
