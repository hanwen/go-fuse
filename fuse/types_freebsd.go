package fuse

import "syscall"

const (
	ENOATTR = Status(syscall.ENOATTR)
	ENODATA = Status(syscall.EIO)
)

const (
	CAP_NO_OPENDIR_SUPPORT = (1 << 24)

	// The higher capabilities are not currently defined by FreeBSD.
	// Ref: https://cgit.freebsd.org/src/tree/sys/fs/fuse/fuse_kernel.h
)

func (o *StatfsOut) FromStatfsT(statfs *syscall.Statfs_t) {
	o.Blocks = statfs.Blocks
	o.Bsize = uint32(statfs.Bsize)
	o.Bfree = statfs.Bfree
	o.Bavail = uint64(statfs.Bavail)
	o.Files = statfs.Files
	o.Ffree = uint64(statfs.Ffree)
	o.Frsize = uint32(statfs.Bsize)
	o.NameLen = uint32(statfs.Namemax)
}
