// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"syscall"
)

const (
	ENOATTR = Status(syscall.ENOATTR) // ENOATTR is not defined for all GOOS.

	// EREMOTEIO is not supported on Darwin.
	EREMOTEIO = Status(syscall.EIO)
)

type Attr struct {
	Ino         uint64
	Size        uint64
	Blocks      uint64
	Atime       uint64
	Mtime       uint64
	Ctime       uint64
	Crtime_     uint64 // OS X
	Atimensec   uint32
	Mtimensec   uint32
	Ctimensec   uint32
	Crtimensec_ uint32 // OS X
	Mode        uint32
	Nlink       uint32
	Owner
	Rdev    uint32
	Flags_  uint32 //  OS X
	Blksize uint32
	Padding uint32
}

const (
	FATTR_CRTIME   = (1 << 28)
	FATTR_CHGTIME  = (1 << 29)
	FATTR_BKUPTIME = (1 << 30)
	FATTR_FLAGS    = (1 << 31)
)

type SetAttrIn struct {
	SetAttrInCommon

	// OS X only
	Bkuptime_    uint64
	Chgtime_     uint64
	Crtime       uint64
	BkuptimeNsec uint32
	ChgtimeNsec  uint32
	CrtimeNsec   uint32
	Flags_       uint32 // see chflags(2)
}

const (
	FOPEN_PURGE_ATTR = (1 << 30)
	FOPEN_PURGE_UBC  = (1 << 31)
)

// compat with linux.
const (
	// Mask for GetAttrIn.Flags. If set, GetAttrIn has a file handle set.
	FUSE_GETATTR_FH = (1 << 0)
)

type GetAttrIn struct {
	InHeader

	Flags_ uint32
	Dummy  uint32
	Fh_    uint64
}

func (g *GetAttrIn) Flags() uint32 {
	return g.Flags_
}

func (g *GetAttrIn) Fh() uint64 {
	return g.Fh_
}

// Uses OpenIn struct for create.
type CreateIn struct {
	InHeader
	Flags uint32

	// Mode for the new file; already takes Umask into account.
	Mode uint32

	// Umask used for this create call.
	Umask   uint32
	Padding uint32
}

type MknodIn struct {
	InHeader

	// Mode to use, including the Umask value
	Mode    uint32
	Rdev    uint32
	Umask   uint32
	Padding uint32
}

type ReadIn struct {
	InHeader
	Fh        uint64
	Offset    uint64
	Size      uint32
	ReadFlags uint32
	LockOwner uint64
	Flags     uint32
	Padding   uint32
}

type WriteIn struct {
	InHeader
	Fh         uint64
	Offset     uint64
	Size       uint32
	WriteFlags uint32
	LockOwner  uint64
	Flags      uint32
	Padding    uint32
}

type SetXAttrIn struct {
	InHeader
	Size     uint32
	Flags    uint32
	Position uint32
	Padding  uint32
}

type GetXAttrIn struct {
	InHeader
	Size     uint32
	Padding  uint32
	Position uint32
	Padding2 uint32
}

const (
	CAP_NODE_RWLOCK      = (1 << 24)
	CAP_RENAME_SWAP      = (1 << 25)
	CAP_RENAME_EXCL      = (1 << 26)
	CAP_ALLOCATE         = (1 << 27)
	CAP_EXCHANGE_DATA    = (1 << 28)
	CAP_CASE_INSENSITIVE = (1 << 29)
	CAP_VOL_RENAME       = (1 << 30)
	CAP_XTIMES           = (1 << 31)
)

type GetxtimesOut struct {
	Bkuptime     uint64
	Crtime       uint64
	Bkuptimensec uint32
	Crtimensec   uint32
}

type ExchangeIn struct {
	InHeader
	Olddir  uint64
	Newdir  uint64
	Options uint64
}

func (s *StatfsOut) FromStatfsT(statfs *syscall.Statfs_t) {
	s.Blocks = statfs.Blocks
	s.Bfree = statfs.Bfree
	s.Bavail = statfs.Bavail
	s.Files = statfs.Files
	s.Ffree = statfs.Ffree
	s.Bsize = uint32(statfs.Iosize) // Iosize translates to Bsize: the optimal transfer size.
	s.Frsize = s.Bsize              // Bsize translates to Frsize: the minimum transfer size.

	// The block counts are in units of statfs.Bsize.
	// If s.Bsize != statfs.Bsize, we have to recalculate the block counts
	// accordingly (s.Bsize is usually 256*statfs.Bsize).
	if s.Bsize > statfs.Bsize {
		adj := uint64(s.Bsize / statfs.Bsize)
		s.Blocks /= adj
		s.Bfree /= adj
		s.Bavail /= adj
	}
}
