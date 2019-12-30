package posixtest

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"testing"
	"unsafe"

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
		err = ioutil.WriteFile(path, nil, 0600)
		if err != nil {
			t.Fatal(err)
		}
	}

	// From C app xfstests/src/t_dir_offset2.c
	const bufSize = 4096
	const historyLen = 1024
	offHistory := make([]int64, historyLen)
	inoHistory := make([]uint64, historyLen)
	buf := make([]byte, bufSize)
	fd, err := syscall.Open(ttt, syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)

	total := 0
	for {
		n, err := unix.Getdents(fd, buf)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		for bpos := 0; bpos < n; total++ {
			d := (*unix.Dirent)(unsafe.Pointer(&buf[bpos]))
			if total > historyLen {
				t.Fatal("too many files")
			}
			for i := 0; i < total; i++ {
				if offHistory[i] == d.Off {
					t.Errorf("entries %d and %d gave duplicate d.Off %d",
						i, total, d.Off)
				}
			}
			offHistory[total] = d.Off
			inoHistory[total] = d.Ino
			bpos += int(d.Reclen)
		}
	}

	// check if seek works correctly
	d := (*unix.Dirent)(unsafe.Pointer(&buf[0]))
	for i := total - 1; i >= 0; i-- {
		var seekTo int64
		if i > 0 {
			seekTo = offHistory[i-1]
		}
		_, err = unix.Seek(fd, seekTo, os.SEEK_SET)
		if err != nil {
			t.Fatal(err)
		}

		n, err := unix.Getdents(fd, buf)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("getdents returned 0 on entry %d", i)
			continue
		}

		if d.Ino != inoHistory[i] {
			t.Errorf("entry %d has inode %d, expected %d",
				i, d.Ino, inoHistory[i])
		}
	}
}
