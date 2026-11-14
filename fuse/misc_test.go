// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"os"
	"syscall"
	"testing"
)

func TestToErrno(t *testing.T) {
	errNo := ToErrno(os.ErrPermission)
	if errNo != EPERM {
		t.Errorf("Wrong conversion %v != %v", errNo, syscall.EPERM)
	}

	e := os.NewSyscallError("syscall", syscall.EPERM)
	errNo = ToErrno(e)
	if errNo != EPERM {
		t.Errorf("Wrong conversion %v != %v", errNo, syscall.EPERM)
	}

	e = os.Remove("this-file-surely-does-not-exist")
	errNo = ToErrno(e)
	if errNo != ENOENT {
		t.Errorf("Wrong conversion %v != %v", errNo, syscall.ENOENT)
	}
}
