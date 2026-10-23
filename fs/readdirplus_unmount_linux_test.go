// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sys/unix"
)

// unmountFS blocks every READDIRPLUS until release is closed, and
// records the opcodes it sees.
type unmountFS struct {
	fuse.RawFileSystem

	mu      sync.Mutex
	ops     []string
	lookups map[string]int

	started chan uint64
	release chan struct{}
}

func (u *unmountFS) record(op string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.ops = append(u.ops, op)
}

func (u *unmountFS) seen(ops ...string) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var r []string
	for _, o := range u.ops {
		for _, want := range ops {
			if o == want {
				r = append(r, o)
			}
		}
	}
	return r
}

func (u *unmountFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	u.mu.Lock()
	u.lookups[name]++
	u.mu.Unlock()
	return u.RawFileSystem.Lookup(ctx, header, name, out)
}

func (u *unmountFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	u.record("OPENDIR")
	return u.RawFileSystem.OpenDir(ctx, input, out)
}

func (u *unmountFS) ReadDirPlus(ctx context.Context, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	u.record("READDIRPLUS")
	u.started <- input.NodeId
	<-u.release
	return u.RawFileSystem.ReadDirPlus(ctx, input, out)
}

func (u *unmountFS) ReleaseDir(ctx context.Context, input *fuse.ReleaseIn) {
	u.record("RELEASEDIR")
	u.RawFileSystem.ReleaseDir(ctx, input)
}

func (u *unmountFS) Forget(nodeid, nlookup uint64) {
	u.record("FORGET")
	u.RawFileSystem.Forget(nodeid, nlookup)
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type unmountEnv struct {
	mnt    string
	ufs    *unmountFS
	server *fuse.Server
	log    *syncBuffer
	dirs   []*Inode

	releaseOnce sync.Once
}

func (e *unmountEnv) releaseHandlers() {
	e.releaseOnce.Do(func() { close(e.ufs.release) })
}

type unmountConfig struct {
	ndirs, nfiles int
	// fuseblk mounts a fuseblk filesystem on this block device.
	fuseblk string
	single  bool
}

// mountUnmountFS mounts dirs "dir0".."dirN" with nfiles entries each.
// The caller unmounts. It skips unless the kernel supports
// NOTIFY_READDIRPLUS.
func mountUnmountFS(t *testing.T, mnt string, cfg unmountConfig) *unmountEnv {
	root := &Inode{}
	e := &unmountEnv{mnt: mnt, log: &syncBuffer{}}
	timeout := time.Minute
	opts := &Options{
		EntryTimeout:    &timeout,
		AttrTimeout:     &timeout,
		NegativeTimeout: &timeout,
		OnAdd: func(ctx context.Context) {
			for d := 0; d < cfg.ndirs; d++ {
				dir := root.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: syscall.S_IFDIR})
				root.AddChild(fmt.Sprintf("dir%d", d), dir, true)
				e.dirs = append(e.dirs, dir)
				for i := 0; i < cfg.nfiles; i++ {
					ch := dir.NewPersistentInode(ctx, &MemRegularFile{}, StableAttr{})
					dir.AddChild(fmt.Sprintf("file-with-a-long-name-%05d", i), ch, true)
				}
			}
		},
	}
	var w io.Writer = e.log
	if testutil.VerboseTest() {
		w = io.MultiWriter(e.log, os.Stderr)
	}
	// Debug logs ENODEV from the device read and all failed reply writes.
	opts.Debug = true
	opts.Logger = log.New(w, "", log.Lmicroseconds)
	opts.MountOptions.Logger = opts.Logger
	opts.MountOptions.SingleThreaded = cfg.single
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR

	e.ufs = &unmountFS{
		RawFileSystem: NewNodeFS(root, opts),
		lookups:       map[string]int{},
		started:       make(chan uint64, 16),
		release:       make(chan struct{}),
	}

	mountPoint := mnt
	if cfg.fuseblk != "" {
		fd, err := syscall.Open("/dev/fuse", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		data := fmt.Sprintf("fd=%d,rootmode=40000,user_id=0,group_id=0,blksize=4096", fd)
		if err := syscall.Mount(cfg.fuseblk, mnt, "fuseblk", 0, data); err != nil {
			syscall.Close(fd)
			t.Fatalf("mount fuseblk: %v", err)
		}
		mountPoint = fmt.Sprintf("/dev/fd/%d", fd)
	}
	server, err := fuse.NewServer(e.ufs, mountPoint, &opts.MountOptions)
	if err != nil {
		t.Fatal(err)
	}
	e.server = server
	go server.Serve()
	if err := server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	if !server.KernelSettings().SupportsNotify(fuse.NOTIFY_READDIRPLUS) {
		e.releaseHandlers()
		runUmount(mnt)
		t.Skip("kernel does not support NOTIFY_READDIRPLUS")
	}
	return e
}

// hint sends NOTIFY_READDIRPLUS for all directories, and waits until
// each pass is blocked in its first READDIRPLUS.
func (e *unmountEnv) hint(t *testing.T) {
	for d, dir := range e.dirs {
		var st syscall.Stat_t
		if err := syscall.Stat(fmt.Sprintf("%s/dir%d", e.mnt, d), &st); err != nil {
			t.Fatal(err)
		}
		if errno := dir.NotifyReaddirplus(); errno != 0 {
			t.Fatalf("NotifyReaddirplus dir%d: %v", d, errno)
		}
	}
	for range e.dirs {
		select {
		case <-e.ufs.started:
		case <-time.After(10 * time.Second):
			t.Fatal("background pass did not send READDIRPLUS")
		}
	}
	if got := len(e.ufs.seen("OPENDIR")); got != len(e.dirs) {
		t.Fatalf("got %d OPENDIRs, want %d", got, len(e.dirs))
	}
}

const umountTimeout = 10 * time.Second

// runUmount runs umount in another process. It does not wait for a
// umount that hangs, as that may be in uninterruptible sleep.
func runUmount(args ...string) (time.Duration, error) {
	start := time.Now()
	cmd := exec.Command("umount", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return time.Since(start), fmt.Errorf("umount %v: %v, %q", args, err, out.String())
		}
		return time.Since(start), nil
	case <-time.After(umountTimeout):
		return time.Since(start), fmt.Errorf("umount %v: still running after %v", args, umountTimeout)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// checkAborted checks the server side of an unmount that aborted the
// connection while the READDIRPLUS handlers were blocked, and then
// releases them.
func (e *unmountEnv) checkAborted(t *testing.T, fuseblk bool) {
	t.Helper()
	waitFor(t, "ENODEV from /dev/fuse", func() bool {
		return strings.Contains(e.log.String(), "received ENODEV")
	})
	e.releaseHandlers()
	done := make(chan struct{})
	go func() {
		e.server.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not exit after releasing the handlers")
	}

	failed := 0
	for _, l := range strings.Split(e.log.String(), "\n") {
		if strings.Contains(l, "Write/Writev failed") && strings.Contains(l, "READDIRPLUS") {
			if !strings.Contains(l, "no such file or directory") {
				t.Errorf("READDIRPLUS reply: got %q, want ENOENT", l)
			}
			failed++
		}
	}
	if failed != len(e.dirs) {
		t.Errorf("got %d failed READDIRPLUS replies, want %d", failed, len(e.dirs))
	}
	if !fuseblk {
		// fuseblk evicts inodes after DESTROY, but before the abort.
		if got := e.ufs.seen("RELEASEDIR", "FORGET"); len(got) > 0 {
			t.Errorf("got %v after abort, want none", got)
		}
	}
}

// checkRemount checks that a new mount at mnt sends LOOKUPs for absent
// names.
func checkRemount(t *testing.T, mnt string) {
	t.Helper()
	e := mountUnmountFS(t, mnt, unmountConfig{ndirs: 1, nfiles: 10})
	defer e.server.Unmount()
	defer e.releaseHandlers()
	var st syscall.Stat_t
	if err := syscall.Stat(mnt+"/dir0/absent", &st); err != syscall.ENOENT {
		t.Fatalf("stat absent after remount: got %v, want ENOENT", err)
	}
	e.ufs.mu.Lock()
	defer e.ufs.mu.Unlock()
	if n := e.ufs.lookups["absent"]; n != 1 {
		t.Errorf("got %d LOOKUPs for absent after remount, want 1", n)
	}
}

func requireRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("need root to unmount from another process")
	}
}

// TestReaddirplusUnmount unmounts while background READDIRPLUS passes
// are blocked in the server.
func TestReaddirplusUnmount(t *testing.T) {
	requireRoot(t)
	for _, tc := range []struct {
		name  string
		ndirs int
		lazy  bool
		abort bool
	}{
		{name: "umount", ndirs: 1},
		{name: "lazy", ndirs: 1, lazy: true},
		{name: "abort", ndirs: 1, abort: true},
		{name: "dirs", ndirs: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mnt := t.TempDir()
			e := mountUnmountFS(t, mnt, unmountConfig{ndirs: tc.ndirs, nfiles: 3000})
			defer e.releaseHandlers()

			var st syscall.Stat_t
			if err := syscall.Stat(mnt, &st); err != nil {
				t.Fatal(err)
			}
			e.hint(t)

			if tc.abort {
				abort := fmt.Sprintf("/sys/fs/fuse/connections/%d/abort", unix.Minor(st.Dev))
				if err := os.WriteFile(abort, []byte("1"), 0); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{mnt}
			if tc.lazy {
				args = []string{"-l", mnt}
			}
			if d, err := runUmount(args...); err != nil {
				t.Fatal(err)
			} else if d > 2*time.Second {
				t.Errorf("umount took %v", d)
			}
			e.checkAborted(t, false)
			checkRemount(t, mnt)
		})
	}
}

const unmountHelperEnv = "GO_FUSE_UNMOUNT_HELPER"

// TestReaddirplusUnmountHelper is the server process for
// TestReaddirplusUnmountKill.
func TestReaddirplusUnmountHelper(t *testing.T) {
	mnt := os.Getenv(unmountHelperEnv)
	if mnt == "" {
		t.Skip("helper for TestReaddirplusUnmountKill")
	}
	e := mountUnmountFS(t, mnt, unmountConfig{ndirs: 1, nfiles: 3000})
	e.hint(t)
	fmt.Println("BLOCKED")
	select {}
}

// TestReaddirplusUnmountKill kills the server with SIGKILL while a
// background READDIRPLUS is blocked in it, and then unmounts.
func TestReaddirplusUnmountKill(t *testing.T) {
	requireRoot(t)
	mnt := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestReaddirplusUnmountHelper$", "-test.v")
	cmd.Env = append(os.Environ(), unmountHelperEnv+"="+mnt)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if l := sc.Text(); l == "BLOCKED" || strings.Contains(l, "SKIP") || strings.Contains(l, "FAIL") {
				result <- l
				break
			}
		}
		io.Copy(io.Discard, stdout)
		close(result)
	}()
	var line string
	select {
	case line = <-result:
	case <-time.After(20 * time.Second):
	}
	if line != "BLOCKED" {
		cmd.Process.Kill()
		cmd.Wait()
		runUmount(mnt)
		if strings.Contains(line, "SKIP") {
			t.Skip("kernel does not support NOTIFY_READDIRPLUS")
		}
		t.Fatalf("helper: got %q, want BLOCKED", line)
	}

	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if d, err := runUmount(mnt); err != nil {
		t.Fatal(err)
	} else if d > 2*time.Second {
		t.Errorf("umount took %v", d)
	}
	checkRemount(t, mnt)
}

// setupLoop attaches a loop device to a new 1 MiB file.
func setupLoop(t *testing.T) string {
	f := t.TempDir() + "/blk"
	if err := os.WriteFile(f, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	ctl, err := unix.Open("/dev/loop-control", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("loop-control: %v", err)
	}
	defer unix.Close(ctl)
	n, err := unix.IoctlRetInt(ctl, unix.LOOP_CTL_GET_FREE)
	if err != nil {
		t.Fatal(err)
	}
	dev := fmt.Sprintf("/dev/loop%d", n)
	lfd, err := unix.Open(dev, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	bfd, err := unix.Open(f, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(bfd)
	if err := unix.IoctlSetInt(lfd, unix.LOOP_SET_FD, bfd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		unix.IoctlSetInt(lfd, unix.LOOP_CLR_FD, 0)
		unix.Close(lfd)
	})
	return dev
}

// TestReaddirplusUnmountFuseblk unmounts a fuseblk filesystem while a
// background READDIRPLUS is blocked. fuseblk waits for the passes
// before sending its synchronous DESTROY, as no request may follow it,
// so umount waits for the blocked handler, whether or not the server
// could answer DESTROY in the meantime.
func TestReaddirplusUnmountFuseblk(t *testing.T) {
	requireRoot(t)
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprintf("single=%v", single), func(t *testing.T) {
			dev := setupLoop(t)
			mnt := t.TempDir()
			e := mountUnmountFS(t, mnt, unmountConfig{ndirs: 1, nfiles: 3000, fuseblk: dev, single: single})
			defer e.releaseHandlers()
			e.hint(t)

			done := make(chan error, 1)
			go func() {
				_, err := runUmount(mnt)
				done <- err
			}()
			select {
			case err := <-done:
				t.Fatalf("umount returned (%v) while the pass was blocked", err)
			case <-time.After(2 * time.Second):
			}
			if strings.Contains(e.log.String(), "DESTROY") {
				t.Fatal("got DESTROY while the pass was blocked")
			}
			e.releaseHandlers()
			if err := <-done; err != nil {
				t.Fatalf("umount after releasing the handler: %v", err)
			}
			e.server.Wait()

			log := e.log.String()
			if i, j := strings.LastIndex(log, "RELEASEDIR"), strings.Index(log, "DESTROY"); i < 0 || j < i {
				t.Errorf("want RELEASEDIR of the pass before DESTROY")
			}
			if strings.Contains(log, "Write/Writev failed") {
				t.Errorf("failed reply writes; want none")
			}
			// The block device is released, so it can be opened exclusively.
			fd, err := unix.Open(dev, unix.O_RDONLY|unix.O_EXCL|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatalf("open %s O_EXCL after umount: %v", dev, err)
			}
			unix.Close(fd)
		})
	}
}

// TestNotifyReaddirplusBusy holds the READDIRPLUS replies for many
// directories, and checks that some hints fail with EBUSY until the
// replies are released.
func TestNotifyReaddirplusBusy(t *testing.T) {
	const ndirs = 32
	e := mountUnmountFS(t, t.TempDir(), unmountConfig{ndirs: ndirs, nfiles: 10})
	defer e.server.Unmount()
	defer e.releaseHandlers()
	go func() {
		for range e.ufs.started {
		}
	}()

	var busy []int
	for d, dir := range e.dirs {
		var st syscall.Stat_t
		if err := syscall.Stat(fmt.Sprintf("%s/dir%d", e.mnt, d), &st); err != nil {
			t.Fatal(err)
		}
		switch errno := dir.NotifyReaddirplus(); errno {
		case 0:
		case syscall.EBUSY:
			busy = append(busy, d)
		default:
			t.Fatalf("NotifyReaddirplus dir%d: %v", d, errno)
		}
	}
	if len(busy) == 0 {
		t.Fatalf("no EBUSY with %d blocked passes", ndirs)
	}

	e.releaseHandlers()
	for _, d := range busy {
		var errno syscall.Errno
		waitFor(t, "passes to finish", func() bool {
			errno = e.dirs[d].NotifyReaddirplus()
			return errno != syscall.EBUSY
		})
		if errno != 0 {
			t.Errorf("NotifyReaddirplus dir%d after release: %v", d, errno)
		}
	}
}
