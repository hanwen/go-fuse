package fuse

import (
	"fmt"
	"github.com/hanwen/go-fuse/raw"
)

func (me *WithFlags) String() string {
	return fmt.Sprintf("File %s (%s) %s %s",
		me.File, me.Description, raw.FlagString(raw.OpenFlagNames, int(me.OpenFlags), "O_RDONLY"),
		raw.FlagString(raw.FuseOpenFlagNames, int(me.FuseFlags), ""))
}

func (a *Attr) String() string {
	return ((*raw.Attr)(a)).String()
}
