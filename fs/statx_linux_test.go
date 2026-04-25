// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package fs

import (
	"os"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/sys/unix"
)

func lstatxPath(p string) (*unix.Statx_t, error) {
	var r unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, p, unix.AT_SYMLINK_NOFOLLOW,
		(unix.STATX_BASIC_STATS | unix.STATX_BTIME |
			unix.STATX_MNT_ID), // unix.STATX_DIOALIGN
		&r)
	return &r, err
}

func clearStatx(st *unix.Statx_t, mask uint32) {
	st.Mask = mask
	if mask&(unix.STATX_TYPE|unix.STATX_MODE) == 0 {
		st.Mode = 0
	}
	if mask&(unix.STATX_NLINK) == 0 {
		st.Nlink = 0
	}
	if mask&(unix.STATX_INO) == 0 {
		st.Ino = 0
	}
	if mask&(unix.STATX_ATIME) == 0 {
		st.Atime = unix.StatxTimestamp{}
	}
	if mask&(unix.STATX_BTIME) == 0 {
		st.Btime = unix.StatxTimestamp{}
	}
	if mask&(unix.STATX_CTIME) == 0 {
		st.Ctime = unix.StatxTimestamp{}
	}
	if mask&(unix.STATX_MTIME) == 0 {
		st.Mtime = unix.StatxTimestamp{}
	}

	st.Dev_minor = 0
	st.Dev_major = 0
	st.Mnt_id = 0
	st.Attributes_mask = 0
}

func TestStatx(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: false, entryCache: false})
	if err := os.WriteFile(tc.origDir+"/file", []byte("blabla"), 0644); err != nil {
		t.Fatal(err)
	}

	oFile := tc.origDir + "/file"
	want, err := lstatxPath(oFile)
	if err != nil {
		t.Fatal(err)
	}
	mFile := tc.mntDir + "/file"
	got, err := lstatxPath(mFile)
	if err != nil {
		t.Fatal(err)
	}
	mask := got.Mask & want.Mask
	clearStatx(got, mask)
	clearStatx(want, mask)
	if diff := pretty.Compare(got, want); diff != "" {
		t.Errorf("got, want: %s", diff)
	}

	// the following works, but does not set Fh in StatxIn
	oFD, err := os.Open(oFile)
	if err != nil {
		t.Fatal(err)
	}
	defer oFD.Close()
	mFD, err := os.Open(mFile)
	if err != nil {
		t.Fatal(err)
	}
	defer mFD.Close()

	var osx, msx unix.Statx_t
	if err := unix.Statx(int(oFD.Fd()), "", unix.AT_EMPTY_PATH,
		(unix.STATX_BASIC_STATS | unix.STATX_BTIME |
			unix.STATX_MNT_ID), // unix.STATX_DIOALIGN
		&osx); err != nil {
		t.Fatal(err)
	}
	if err := unix.Statx(int(mFD.Fd()), "", unix.AT_EMPTY_PATH,
		(unix.STATX_BASIC_STATS | unix.STATX_BTIME |
			unix.STATX_MNT_ID), // unix.STATX_DIOALIGN
		&msx); err != nil {
		t.Fatal(err)
	}

	mask = msx.Mask & osx.Mask
	clearStatx(&msx, mask)
	clearStatx(&osx, mask)
	if diff := pretty.Compare(msx, osx); diff != "" {
		t.Errorf("got, want: %s", diff)
	}
}
