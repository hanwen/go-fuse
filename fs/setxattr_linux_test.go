// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
	"golang.org/x/sys/unix"
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

// minimalACL encodes user::rwx group::rwx other::---.
func minimalACL() []byte {
	b := binary.LittleEndian.AppendUint32(nil, 2)
	for _, e := range []struct{ tag, perm uint16 }{{0x01, 7}, {0x04, 7}, {0x20, 0}} {
		b = binary.LittleEndian.AppendUint16(b, e.tag)
		b = binary.LittleEndian.AppendUint16(b, e.perm)
		b = binary.LittleEndian.AppendUint32(b, 0xffffffff)
	}
	return b
}

func TestSetXAttrKillSgid(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root")
	}
	const uid, gid = 65534, 12345

	orig := t.TempDir()
	mnt := t.TempDir()
	for _, d := range []string{filepath.Dir(orig), orig, filepath.Dir(mnt), mnt} {
		if err := os.Chmod(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	origFile := filepath.Join(orig, "file")
	if err := os.WriteFile(origFile, nil, 0770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(origFile, uid, gid); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(origFile, 0770|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if err := unix.Lsetxattr(origFile, "system.posix_acl_access", minimalACL(), 0); err != nil {
		t.Skipf("underlying filesystem has no ACLs: %v", err)
	}

	root, err := NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	opts := &Options{}
	opts.Debug = testutil.VerboseTest()
	opts.AllowOther = true
	opts.EnableAcl = true
	opts.ExtraCapabilities = fuse.CAP_SETXATTR_EXT
	srv, err := Mount(mnt, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	if srv.KernelSettings().Flags64()&fuse.CAP_SETXATTR_EXT == 0 {
		t.Skip("kernel does not support CAP_SETXATTR_EXT")
	}

	// setfsuid is per thread; the locked thread exits with the goroutine.
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		unix.Setfsgid(uid)
		unix.Setfsuid(uid)
		errc <- unix.Setxattr(filepath.Join(mnt, "file"), "system.posix_acl_access", minimalACL(), 0)
	}()
	if err := <-errc; err != nil {
		t.Fatalf("Setxattr: %v", err)
	}

	var st syscall.Stat_t
	if err := syscall.Lstat(origFile, &st); err != nil {
		t.Fatal(err)
	}
	if st.Mode&syscall.S_ISGID != 0 {
		t.Errorf("mode %o: setgid not cleared", st.Mode)
	}
	if got := st.Mode & 0777; got != 0770 {
		t.Errorf("mode %o, want 0770", got)
	}
}
