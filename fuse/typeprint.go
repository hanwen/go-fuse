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

func (a *Attr) String() string {
	return ((*raw.Attr)(a)).String()
}
