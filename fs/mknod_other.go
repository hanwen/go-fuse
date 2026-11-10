// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd

package fs

import "syscall"

func mknod(path string, mode, rdev uint32) error {
	return syscall.Mknod(path, mode, intDev(rdev))
}
