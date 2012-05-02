package fuse

import (
	"fmt"
	"github.com/hanwen/go-fuse/raw"
)

var writeFlagNames map[int]string
var readFlagNames map[int]string

func init() {
	writeFlagNames = map[int]string{
		WRITE_CACHE:     "CACHE",
		WRITE_LOCKOWNER: "LOCKOWNER",
	}
	readFlagNames = map[int]string{
		READ_LOCKOWNER: "LOCKOWNER",
	}
}

func (a *Attr) String() string {
	return fmt.Sprintf(
		"{M0%o S=%d L=%d "+
			"%d:%d "+
			"%d*%d %d:%d "+
			"A %d.%09d "+
			"M %d.%09d "+
			"C %d.%09d}",
		a.Mode, a.Size, a.Nlink,
		a.Uid, a.Gid,
		a.Blocks, a.Blksize,
		a.Rdev, a.Ino, a.Atime, a.Atimensec, a.Mtime, a.Mtimensec,
		a.Ctime, a.Ctimensec)
}

func (me *ReadIn) String() string {
	return fmt.Sprintf("{Fh %d off %d sz %d %s L %d %s}",
		me.Fh, me.Offset, me.Size,
		raw.FlagString(readFlagNames, int(me.ReadFlags), ""),
		me.LockOwner,
		raw.FlagString(raw.OpenFlagNames, int(me.Flags), "RDONLY"))
}

func (me *WithFlags) String() string {
	return fmt.Sprintf("File %s (%s) %s %s",
		me.File, me.Description, raw.FlagString(raw.OpenFlagNames, int(me.OpenFlags), "O_RDONLY"),
		raw.FlagString(raw.FuseOpenFlagNames, int(me.FuseFlags), ""))
}


