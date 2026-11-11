// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin

package fuse

import "golang.org/x/sys/unix"

func writev(fd int, packet [][]byte) (n int, err error) {
	err = handleEINTR(func() error {
		var err error
		n, err = unix.Writev(fd, packet)
		return err
	})
	return n, err
}
