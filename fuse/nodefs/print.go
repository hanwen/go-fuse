package nodefs

import (
	"fmt"

	"github.com/hanwen/go-fuse/fuse"
)

func (me *WithFlags) String() string {
	return fmt.Sprintf("File %s (%s) %s %s",
		me.File, me.Description, fuse.FlagString(fuse.OpenFlagNames, int64(me.OpenFlags), "O_RDONLY"),
		fuse.FlagString(fuse.FuseOpenFlagNames, int64(me.FuseFlags), ""))
}
