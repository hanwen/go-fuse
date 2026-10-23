// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !openbsd && !darwin

package fs

import "golang.org/x/sys/unix"

func (n *LoopbackNode) mknodat(dirfd int, name string, mode, rdev uint32) error {
	return unix.Mknodat(dirfd, name, mode, intDev(rdev))
}
