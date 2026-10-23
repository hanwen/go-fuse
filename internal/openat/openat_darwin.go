// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openat

import (
	"golang.org/x/sys/unix"
)

const (
	searchFlags = unix.O_RDONLY
	symlinkFlag = unix.O_SYMLINK
)
