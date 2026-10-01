// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "golang.org/x/sys/unix"

func (r *readResultFd) Readv(dst [][]byte) (int, Status) {
	dst = sliceIov(dst, 0, r.Sz)
	var n int
	var err error
	if r.Off == -1 {
		n, err = unix.Readv(int(r.Fd), dst)
	} else {
		n, err = unix.Preadv(int(r.Fd), dst, r.Off)
	}
	if n < 0 {
		n = 0
	}
	return n, ToStatus(err)
}
