// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !freebsd

package posixtest

import "golang.org/x/sys/unix"

func clearStatRevision(st *unix.Stat_t) {
}
