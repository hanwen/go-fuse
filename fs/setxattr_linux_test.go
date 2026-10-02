// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"log"
	"os"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
)

// With CAP_SETXATTR_EXT, SetXAttrIn is 8 bytes longer; a wrong size
// shifts the attribute name.
func TestSetXAttrExt(t *testing.T) {
	orig := t.TempDir()
	mnt := t.TempDir()
	root, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	opts := &Options{}
	opts.Debug = testutil.VerboseTest()
	opts.Logger = log.New(os.Stderr, "", 0)
	opts.ExtraCapabilities = fuse.CAP_SETXATTR_EXT
	srv, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	if srv.KernelSettings().Flags64()&fuse.CAP_SETXATTR_EXT == 0 {
		t.Skip("kernel does not support CAP_SETXATTR_EXT")
	}
	posixtest.XAttr(t, mnt)
}
