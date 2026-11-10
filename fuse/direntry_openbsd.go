// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

// Like syscall.Dirent, but without the [256]byte name.
type dirent struct {
	Ino    uint64
	Off    int64
	Reclen uint16
	Type   uint8
	Namlen uint8
	Pad    [4]uint8
	Name   [1]int8
}

func (de *dirent) nameLength() int {
	return int(de.Namlen)
}
