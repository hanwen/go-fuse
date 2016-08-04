// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"syscall"
)

func clearStatfs(s *syscall.Statfs_t) {
	empty := syscall.Statfs_t{}

	// FUSE can't set the following fields defined in struct vfsstatfs on darwin.
	s.Type = empty.Type
	s.Fsid = empty.Fsid
	s.Owner = empty.Owner
	s.Flags = empty.Flags
	s.Fssubtype = empty.Fssubtype
	s.Fstypename = empty.Fstypename
	s.Mntonname = empty.Mntonname
	s.Mntfromname = empty.Mntfromname
}
