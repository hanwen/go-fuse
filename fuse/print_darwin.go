// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
)

func init() {
	initFlagNames.set(CAP_NODE_RWLOCK, "NODE_RWLOCK")
	initFlagNames.set(CAP_RENAME_SWAP, "RENAME_SWAP")
	initFlagNames.set(CAP_RENAME_EXCL, "RENAME_EXCL")
	initFlagNames.set(CAP_ALLOCATE, "ALLOCATE")
	initFlagNames.set(CAP_EXCHANGE_DATA, "EXCHANGE_DATA")
	initFlagNames.set(CAP_XTIMES, "XTIMES")
	initFlagNames.set(CAP_VOL_RENAME, "VOL_RENAME")
	initFlagNames.set(CAP_CASE_INSENSITIVE, "CASE_INSENSITIVE")
}

func (in *CreateIn) string() string {
	return fmt.Sprintf(
		"{0%o [%s]}", in.Mode,
		flagString(openFlagNames, int64(in.Flags), "O_RDONLY"))
}

func (in *GetAttrIn) string() string { return "" }

func (a *Attr) flagsString() string { return "" }

func (in *SetXAttrIn) string() string {
	return fmt.Sprintf("{sz %d f%o}", in.Size, in.Flags)
}

func (in *MknodIn) string() string {
	return fmt.Sprintf("{0%o, %d}", in.Mode, in.Rdev)
}

func (in *ReadIn) string() string {
	return fmt.Sprintf("{Fh %d [%d +%d) %s}",
		in.Fh, in.Offset, in.Size,
		flagString(readFlagNames, int64(in.ReadFlags), ""))
}

func (in *WriteIn) string() string {
	return fmt.Sprintf("{Fh %d [%d +%d) %s}",
		in.Fh, in.Offset, in.Size,
		flagString(writeFlagNames, int64(in.WriteFlags), ""))
}
