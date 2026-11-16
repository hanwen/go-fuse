// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package posixtest

import "golang.org/x/sys/unix"

// uncachedMtime returns the mtime of path. Without a way to bypass the
// attribute cache, it may be stale.
func uncachedMtime(path string) (sec, nsec int64, err error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, 0, err
	}
	return int64(st.Mtim.Sec), int64(st.Mtim.Nsec), nil
}
