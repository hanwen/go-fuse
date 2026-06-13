// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// TestIoUringSmoke mounts a trivial loopback fs with EnableIoUring and
// runs a stat through it. Skips if the kernel does not advertise
// CAP_OVER_IO_URING; that capability is the gate for the whole
// transport.
func TestIoUringSmoke(t *testing.T) {
	src := t.TempDir()
	mnt := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, "hello"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	root, err := NewLoopbackRoot(src)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := Mount(mnt, root, &Options{
		MountOptions: fuse.MountOptions{
			EnableIoUring: true,
			Debug:         testutil.VerboseTest(),
		},
	})
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	defer srv.Unmount()

	if srv.KernelSettings().Flags64()&fuse.CAP_OVER_IO_URING == 0 {
		t.Skip("kernel does not advertise CAP_OVER_IO_URING")
	}

	var st syscall.Stat_t
	if err := syscall.Stat(filepath.Join(mnt, "hello"), &st); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Size != 2 {
		t.Errorf("size = %d, want 2", st.Size)
	}
}
