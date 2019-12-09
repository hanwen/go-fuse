// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"os/user"
	"strconv"
	"testing"
)

func TestHasAccess(t *testing.T) {
	type testcase struct {
		uid, gid, fuid, fgid uint32
		perm, mask           uint32
		want                 bool
	}

	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}

	myIntId, _ := strconv.Atoi(u.Uid)
	myUid := uint32(myIntId)
	myIntGid, _ := strconv.Atoi(u.Gid)
	myGid := uint32(myIntGid)

	gids, err := u.GroupIds()
	if err != nil {
		t.Fatalf("user.GroupIds: %v", err)
	}

	var myOtherGid, notMyGid uint32
	mygids := map[uint32]bool{}

	for _, g := range gids {
		gnum, _ := strconv.Atoi(g)
		gnum32 := uint32(gnum)
		mygids[gnum32] = true
		if g != u.Gid {
			myOtherGid = uint32(gnum)
		}
	}

	for i := uint32(1); i < 1000; i++ {
		if !mygids[i] {
			notMyGid = i
			break
		}
	}

	_ = myOtherGid
	_ = notMyGid
	cases := []testcase{
		{myUid, myGid, myUid, myGid, 0100, 01, true},
		{myUid, myGid, myUid + 1, notMyGid, 0001, 0001, true},
		{myUid, myGid, myUid + 1, notMyGid, 0000, 0001, false},
		{myUid, myGid, myUid + 1, notMyGid, 0007, 0000, true},
		{myUid, myGid, myUid + 1, notMyGid, 0020, 002, false},
		{myUid, myGid, myUid, myGid, 0000, 01, false},
		{myUid, myGid, myUid, myGid, 0200, 01, false},
		{0, myGid, myUid + 1, notMyGid, 0700, 01, true},
	}

	if myOtherGid != 0 {
		cases = append(cases, testcase{myUid, myGid, myUid + 1, myOtherGid, 0020, 002, true})
	}
	for i, tc := range cases {
		got := HasAccess(tc.uid, tc.gid, tc.fuid, tc.fgid, tc.perm, tc.mask)
		if got != tc.want {
			t.Errorf("%d: accessCheck(%v): got %v, want %v", i, tc, got, tc.want)
		}
	}
}
