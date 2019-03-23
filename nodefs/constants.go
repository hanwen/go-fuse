// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

// OK is the Errno return value to indicate absense of errors.
var OK = syscall.Errno(0)

func ToErrno(err error) syscall.Errno {
	s := fuse.ToStatus(err)
	return syscall.Errno(s)
}

// RENAME_EXCHANGE is a flag argument for renameat2()
const RENAME_EXCHANGE = 0x2
