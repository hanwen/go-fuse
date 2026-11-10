// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd || openbsd

package fuse

const (
	// CAP_RENAME_SWAP only exists on macOS.
	CAP_RENAME_SWAP = 0x0

	CAP_EXPLICIT_INVAL_DATA = 0x0
	CAP_SETXATTR_EXT        = 0x0
)

func (o *InitOut) setFlags(flags uint64) {
	o.Flags = uint32(flags)
}
