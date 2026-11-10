// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "syscall"

const (
	ENOATTR = Status(syscall.ENOATTR)
	ENODATA = Status(syscall.ENOATTR)
)

func (s *StatfsOut) FromStatfsT(statfs *syscall.Statfs_t) {
	s.Blocks = statfs.F_blocks
	s.Bsize = statfs.F_bsize
	s.Bfree = statfs.F_bfree
	s.Bavail = uint64(statfs.F_bavail)
	s.Files = statfs.F_files
	s.Ffree = statfs.F_ffree
	s.Frsize = statfs.F_bsize
	s.NameLen = statfs.F_namemax
}
