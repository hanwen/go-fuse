// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import (
	"syscall"
	"testing"
)

func TestNewServerFromDevIncompatible(t *testing.T) {
	compatible := func() *Dev {
		fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { syscall.Close(fds[1]) })
		d := &Dev{fd: fds[0]}
		d.in.Major = _FUSE_KERNEL_VERSION
		d.in.Minor = _OUR_MINOR_VERSION
		d.out.Major = _FUSE_KERNEL_VERSION
		d.out.Minor = _OUR_MINOR_VERSION
		return d
	}
	if err := checkSession(&compatible().out); err != nil {
		t.Fatalf("compatible session: %v", err)
	}

	for name, mod := range map[string]func(d *Dev){
		"major":      func(d *Dev) { d.out.Major++ },
		"minor":      func(d *Dev) { d.out.Minor = _OUR_MINOR_VERSION + 1 },
		"capability": func(d *Dev) { d.out.Flags2 |= 1 << 31 },
	} {
		t.Run(name, func(t *testing.T) {
			d := compatible()
			mod(d)
			if _, err := NewServerFromDev(&defaultRawFileSystem{}, d, nil, nil); err == nil {
				t.Fatal("NewServerFromDev succeeded")
			}
			if d.fd != -1 {
				t.Error("device not closed")
			}
		})
	}
}
