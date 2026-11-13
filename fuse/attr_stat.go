// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"os"
	"syscall"
)

func ToStatT(f os.FileInfo) *syscall.Stat_t {
	s, _ := f.Sys().(*syscall.Stat_t)
	if s != nil {
		return s
	}
	return nil
}

func ToAttr(f os.FileInfo) *Attr {
	if f == nil {
		return nil
	}
	s := ToStatT(f)
	if s != nil {
		a := &Attr{}
		a.FromStat(s)
		return a
	}
	return nil
}
