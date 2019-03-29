// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"syscall"
	"testing"
)

func TestReadonlyCreate(t *testing.T) {
	root := &Inode{}

	mntDir, clean := testMount(t, root, nil)
	defer clean()

	_, err := syscall.Creat(mntDir+"/test", 0644)
	if want := syscall.EROFS; want != err {
		t.Fatalf("got err %v, want %v", err, want)
	}
}
