// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd

package posixtest

import "golang.org/x/sys/unix"

const (
	seekData    = unix.SEEK_DATA
	seekHole    = unix.SEEK_HOLE
	kernelLocks = false
)

var (
	getxattr    = unix.Getxattr
	setxattr    = unix.Setxattr
	listxattr   = unix.Listxattr
	removexattr = unix.Removexattr
)
