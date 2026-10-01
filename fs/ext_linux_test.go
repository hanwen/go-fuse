// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

type extTestCase struct {
	origDir string
	mntDir  string
	server  *fuse.Server
	logBuf  bytes.Buffer
}

func mountExt(t *testing.T, capability uint64, setup func(origDir string)) *extTestCase {
	tc := &extTestCase{
		origDir: t.TempDir(),
		mntDir:  t.TempDir(),
	}
	if setup != nil {
		setup(tc.origDir)
	}
	root, err := NewLoopbackRoot(tc.origDir)
	if err != nil {
		t.Fatal(err)
	}

	var w io.Writer = &tc.logBuf
	if testutil.VerboseTest() {
		w = io.MultiWriter(w, os.Stderr)
	}
	logger := log.New(w, "", 0)
	opts := &Options{
		Logger: logger,
	}
	opts.Debug = true
	opts.MountOptions.Logger = logger
	opts.ExtraCapabilities = capability

	tc.server, err = Mount(tc.mntDir, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tc.server.Unmount()
	})
	if tc.server.KernelSettings().Flags64()&capability == 0 {
		t.Skipf("kernel does not support capability %x", capability)
	}
	return tc
}

func (tc *extTestCase) unmount(t *testing.T) string {
	if err := tc.server.Unmount(); err != nil {
		t.Fatal(err)
	}
	return tc.logBuf.String()
}

func createAll(t *testing.T, tc *extTestCase, subdir string) {
	const target = "some/symlink/target"
	mnt := filepath.Join(tc.mntDir, subdir)
	orig := filepath.Join(tc.origDir, subdir)

	if err := os.Symlink(target, filepath.Join(mnt, "symlink")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := os.Mkdir(filepath.Join(mnt, "dir"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(mnt, "fifo"), 0644); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(mnt, "file"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.Close()

	for _, d := range []string{mnt, orig} {
		if got, err := os.Readlink(filepath.Join(d, "symlink")); err != nil {
			t.Errorf("Readlink: %v", err)
		} else if got != target {
			t.Errorf("Readlink(%s): got %q, want %q", d, got, target)
		}
		for name, mode := range map[string]os.FileMode{
			"symlink": os.ModeSymlink,
			"dir":     os.ModeDir,
			"fifo":    os.ModeNamedPipe,
			"file":    0,
		} {
			fi, err := os.Lstat(filepath.Join(d, name))
			if err != nil {
				t.Errorf("Lstat: %v", err)
			} else if fi.Mode().Type() != mode {
				t.Errorf("Lstat(%s/%s): got type %v, want %v", d, name, fi.Mode().Type(), mode)
			}
		}
	}
}

var createOps = []string{"SYMLINK", "MKDIR", "MKNOD", "CREATE"}

func extLines(debugLog string, op string) []string {
	var r []string
	for _, l := range strings.Split(debugLog, "\n") {
		if strings.HasPrefix(l, "rx ") && strings.Contains(l, ": "+op+" ") {
			r = append(r, l)
		}
	}
	return r
}

func TestExtSuppGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	gid := -1
	for _, g := range groups {
		if g != os.Getegid() {
			gid = g
			break
		}
	}
	if gid < 0 {
		t.Skip("need a supplementary group")
	}

	tc := mountExt(t, fuse.CAP_CREATE_SUPP_GROUP, func(orig string) {
		sub := filepath.Join(orig, "sub")
		if err := os.Mkdir(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(sub, -1, gid); err != nil {
			t.Skipf("Chown to group %d: %v", gid, err)
		}
	})

	createAll(t, tc, "sub")
	debugLog := tc.unmount(t)

	want := fmt.Sprintf("ext{groups=[%d]}", gid)
	for _, op := range createOps {
		lines := extLines(debugLog, op)
		if len(lines) == 0 {
			t.Errorf("no %s requests in log", op)
		}
		for _, l := range lines {
			if !strings.Contains(l, want) {
				t.Errorf("%s request does not contain %q: %s", op, want, l)
			}
		}
	}
}

func lsmActive() bool {
	_, err := os.Stat("/sys/fs/selinux/enforce")
	return err == nil
}

func TestExtSecurityCtx(t *testing.T) {
	tc := mountExt(t, fuse.CAP_SECURITY_CTX, nil)
	createAll(t, tc, "")
	debugLog := tc.unmount(t)

	for _, op := range createOps {
		lines := extLines(debugLog, op)
		if len(lines) == 0 {
			t.Errorf("no %s requests in log", op)
		}
		for _, l := range lines {
			if !strings.Contains(l, " ext{") {
				t.Errorf("%s request has no extension: %s", op, l)
			}
			if lsmActive() && !strings.Contains(l, "secctx=[security.selinux:") {
				t.Errorf("%s request has no SELinux context: %s", op, l)
			}
		}
	}
}
