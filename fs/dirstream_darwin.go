//go:build !darwin

// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

func getdents(fd uintptr, buf []byte) (int, error) {
	return unix.Getdirentries(fd, buf, nil)
}
