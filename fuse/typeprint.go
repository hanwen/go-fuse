package fuse

import (
	"fmt"
	"github.com/hanwen/go-fuse/raw"
)

var initFlagNames map[int]string
var writeFlagNames map[int]string
var readFlagNames map[int]string
var accessFlagName map[int]string

func init() {
	initFlagNames = map[int]string{
		CAP_ASYNC_READ:     "ASYNC_READ",
		CAP_POSIX_LOCKS:    "POSIX_LOCKS",
		CAP_FILE_OPS:       "FILE_OPS",
		CAP_ATOMIC_O_TRUNC: "ATOMIC_O_TRUNC",
		CAP_EXPORT_SUPPORT: "EXPORT_SUPPORT",
		CAP_BIG_WRITES:     "BIG_WRITES",
		CAP_DONT_MASK:      "DONT_MASK",
		CAP_SPLICE_WRITE:   "SPLICE_WRITE",
		CAP_SPLICE_MOVE:    "SPLICE_MOVE",
		CAP_SPLICE_READ:    "SPLICE_READ",
	}
	writeFlagNames = map[int]string{
		WRITE_CACHE:     "CACHE",
		WRITE_LOCKOWNER: "LOCKOWNER",
	}
	readFlagNames = map[int]string{
		READ_LOCKOWNER: "LOCKOWNER",
	}
	accessFlagName = map[int]string{
		X_OK: "x",
		W_OK: "w",
		R_OK: "r",
	}
}

func (me *Attr) String() string {
	return fmt.Sprintf(
		"{M0%o S=%d L=%d "+
			"%d:%d "+
			"%d*%d %d:%d "+
			"A %d.%09d "+
			"M %d.%09d "+
			"C %d.%09d}",
		me.Mode, me.Size, me.Nlink,
		me.Uid, me.Gid,
		me.Blocks, me.Blksize,
		me.Rdev, me.Ino, me.Atime, me.Atimensec, me.Mtime, me.Mtimensec,
		me.Ctime, me.Ctimensec)
}

func (me *AttrOut) String() string {
	return fmt.Sprintf(
		"{A%d.%09d %v}",
		me.AttrValid, me.AttrValidNsec, &me.Attr)
}

func (me *CreateIn) String() string {
	return fmt.Sprintf(
		"{0%o [%s] (0%o)}", me.Mode,
		raw.FlagString(raw.OpenFlagNames, int(me.Flags), "O_RDONLY"), me.Umask)
}

func (me *EntryOut) String() string {
	return fmt.Sprintf("{%d E%d.%09d A%d.%09d %v}",
		me.NodeId, me.EntryValid, me.EntryValidNsec,
		me.AttrValid, me.AttrValidNsec, &me.Attr)
}

func (me *CreateOut) String() string {
	return fmt.Sprintf("{%v %v}", &me.EntryOut, &me.OpenOut)
}

func (me *InitIn) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s}",
		me.Major, me.Minor, me.MaxReadAhead,
		raw.FlagString(initFlagNames, int(me.Flags), ""))
}
func (me *InitOut) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s %d/%d Wr 0x%x}",
		me.Major, me.Minor, me.MaxReadAhead,
		raw.FlagString(initFlagNames, int(me.Flags), ""),
		me.CongestionThreshold, me.MaxBackground, me.MaxWrite)
}

func (me *ReadIn) String() string {
	return fmt.Sprintf("{Fh %d off %d sz %d %s L %d %s}",
		me.Fh, me.Offset, me.Size,
		raw.FlagString(readFlagNames, int(me.ReadFlags), ""),
		me.LockOwner,
		raw.FlagString(raw.OpenFlagNames, int(me.Flags), "RDONLY"))
}


func (me *FlushIn) String() string {
	return fmt.Sprintf("{Fh %d}", me.Fh)
}

func (me *AccessIn) String() string {
	return fmt.Sprintf("{%s}", raw.FlagString(accessFlagName, int(me.Mask), ""))
}

func (me *SetXAttrIn) String() string {
	return fmt.Sprintf("{sz %d f%o}", me.Size, me.Flags)
}

func (me *GetXAttrIn) String() string {
	return fmt.Sprintf("{sz %d}", me.Size)
}

func (me *GetXAttrOut) String() string {
	return fmt.Sprintf("{sz %d}", me.Size)
}

func (me *Kstatfs) String() string {
	return fmt.Sprintf(
		"{b%d f%d fs%d ff%d bs%d nl%d frs%d}",
		me.Blocks, me.Bfree, me.Bavail, me.Files, me.Ffree,
		me.Bsize, me.NameLen, me.Frsize)
}

func (me *WithFlags) String() string {
	return fmt.Sprintf("File %s (%s) %s %s",
		me.File, me.Description, raw.FlagString(raw.OpenFlagNames, int(me.OpenFlags), "O_RDONLY"),
		raw.FlagString(raw.FuseOpenFlagNames, int(me.FuseFlags), ""))
}


