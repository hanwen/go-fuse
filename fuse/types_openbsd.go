// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "syscall"

const (
	ENOATTR = syscall.ENOATTR
	ENODATA = syscall.ENOATTR
)

func (o *StatfsOut) FromStatfsT(statfs *syscall.Statfs_t) {
	o.Blocks = statfs.F_blocks
	o.Bsize = statfs.F_bsize
	o.Bfree = statfs.F_bfree
	o.Bavail = uint64(statfs.F_bavail)
	o.Files = statfs.F_files
	o.Ffree = statfs.F_ffree
	o.Frsize = statfs.F_bsize
	o.NameLen = statfs.F_namemax
}
