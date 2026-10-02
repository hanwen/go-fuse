//go:build !darwin

package fuse

import "unsafe"

type Attr struct {
	Ino  uint64
	Size uint64

	// Blocks is the number of 512-byte blocks that the file occupies on disk.
	Blocks    uint64
	Atime     uint64
	Mtime     uint64
	Ctime     uint64
	Atimensec uint32
	Mtimensec uint32
	Ctimensec uint32
	Mode      uint32
	Nlink     uint32
	Owner
	Rdev uint32

	// Blksize is the preferred size for file system operations.
	Blksize uint32

	Flags uint32
}

const (
	// Attr.Flags: directory is the root of a server-side submount; virtio-fs automounts it (CAP_SUBMOUNTS).
	ATTR_SUBMOUNT = (1 << 0)
	// Attr.Flags: map the file through the virtio-fs DAX window (CAP_HAS_INODE_DAX, dax=inode).
	ATTR_DAX = (1 << 1)
)

type SetAttrIn struct {
	SetAttrInCommon
}

type SetXAttrIn struct {
	InHeader
	Size  uint32
	Flags uint32

	SetXAttrFlags uint32
	Padding       uint32
}

const (
	// SetXAttrIn.SetXAttrFlags: setting system.posix_acl_access must clear sgid.
	SETXATTR_ACL_KILL_SGID = (1 << 0)
)

const compatSetXAttrInSize = int(unsafe.Sizeof(InHeader{})) + 8

func setXAttrInSize(negotiated uint64) int {
	if negotiated&CAP_SETXATTR_EXT == 0 {
		return compatSetXAttrInSize
	}
	return int(unsafe.Sizeof(SetXAttrIn{}))
}

type GetXAttrIn struct {
	InHeader
	Size    uint32
	Padding uint32
}
