// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package winfsp

import "syscall"

func statusFromSystemError(errno syscall.Errno) (ntStatus, bool) {
	return 0, false
}
