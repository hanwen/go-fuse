// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fallocate

import "golang.org/x/sys/unix"

// OpenBSD has no fallocate or posix_fallocate.
func fallocate(fd int, mode uint32, off int64, len int64) error {
	return unix.EOPNOTSUPP
}
