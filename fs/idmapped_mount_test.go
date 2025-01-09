// Copyright 2025 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

func TestIDMappedMount(t *testing.T) {
	// sys_open_tree requires CAP_SYS_ADMIN
	if os.Geteuid() != 0 {
		t.Skip("id-mapped mount requires CAP_SYS_ADMIN")
	}

	tc := newTestCase(t, &testOptions{idMappedMount: true})
	tc.writeOrig("file", "hello", 0644)

	fi, err := os.Lstat(filepath.Join(tc.origDir, "file"))
	if err != nil {
		t.Fatalf("stat for path %s failed: %v", filepath.Join(tc.origDir, "file"), err)
	}
	st := fuse.ToStatT(fi)

	if tc.server.KernelSettings().Flags64()&fuse.CAP_ALLOW_IDMAP == 0 {
		t.Skip("Kernel does not support id-mapped mount")
	}

	const offset = 10000
	fd, err := usernsFD(offset)
	if err != nil {
		t.Fatalf("failed to get user namespace FD: %v", err)
	}
	defer fd.Close()

	idDir := t.TempDir()
	if err = idMapMount(tc.mntDir, idDir, int(fd.Fd())); err != nil {
		t.Fatalf("id-mapped mount failed: %v", err)
	}
	defer unix.Unmount(idDir, 0)

	mfi, err := os.Lstat(filepath.Join(idDir, "file"))
	if err != nil {
		t.Fatalf("stat for path %s failed: %v", filepath.Join(idDir, "file"), err)
	}
	mst := fuse.ToStatT(mfi)

	if st.Uid+offset != mst.Uid {
		t.Errorf("Uid %v + offset %v != mapped Uid %v", st.Uid, offset, mst.Uid)
	}
	if st.Gid+offset != mst.Gid {
		t.Errorf("Gid %v + offset %v != mapped Gid %v", st.Gid, offset, mst.Gid)
	}
}

func idMapMount(source, target string, fd int) (err error) {
	dFd, err := unix.OpenTree(-int(unix.EBADF), source, uint(unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC|unix.AT_EMPTY_PATH))
	if err != nil {
		return fmt.Errorf("open tree failed %s: %w", source, err)
	}
	defer unix.Close(dFd)
	if err = unix.MountSetattr(dFd, "", unix.AT_EMPTY_PATH, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_IDMAP, Userns_fd: uint64(fd)}); err != nil {
		return fmt.Errorf("set attr for %s failed: %w", source, err)
	}
	if err = unix.MoveMount(dFd, "", -int(unix.EBADF), target, unix.MOVE_MOUNT_F_EMPTY_PATH); err != nil {
		return fmt.Errorf("move mount to %s failed: %w", target, err)
	}
	return nil
}

func usernsFD(offset int) (*os.File, error) {
	var err error
	args := []string{"sleep", "1h"}
	if args[0], err = exec.LookPath("sleep"); err != nil {
		return nil, fmt.Errorf("failed to find sleep binary: %w", err)
	}
	p, err := os.StartProcess(args[0], args, &os.ProcAttr{
		Sys: &syscall.SysProcAttr{
			Cloneflags: unix.CLONE_NEWUSER,
			UidMappings: []syscall.SysProcIDMap{
				{
					ContainerID: 0,
					HostID:      offset,
					Size:        offset,
				},
			},
			GidMappings: []syscall.SysProcIDMap{
				{
					ContainerID: 0,
					HostID:      offset,
					Size:        offset,
				},
			},
			Pdeathsig: syscall.SIGKILL,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}
	defer func() {
		p.Kill()
		p.Wait()
	}()
	return os.Open(fmt.Sprintf("/proc/%d/ns/user", p.Pid))
}
