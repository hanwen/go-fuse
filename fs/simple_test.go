// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"github.com/hanwen/go-fuse/v2/posixtest"
)

type testCase struct {
	*testing.T

	dir     string
	origDir string
	mntDir  string

	loopback InodeEmbedder
	rawFS    fuse.RawFileSystem
	server   *fuse.Server
}

// writeOrig writes a file into the backing directory of the loopback mount
func (tc *testCase) writeOrig(path, content string, mode os.FileMode) {
	if err := ioutil.WriteFile(filepath.Join(tc.origDir, path), []byte(content), mode); err != nil {
		tc.Fatal(err)
	}
}

func (tc *testCase) Clean() {
	if err := tc.server.Unmount(); err != nil {
		tc.Fatal(err)
	}
	if err := os.RemoveAll(tc.dir); err != nil {
		tc.Fatal(err)
	}
}

type testOptions struct {
	entryCache    bool
	attrCache     bool
	suppressDebug bool
	testDir       string
	ro            bool
}

// newTestCase creates the directories `orig` and `mnt` inside a temporary
// directory and mounts a loopback filesystem, backed by `orig`, on `mnt`.
func newTestCase(t *testing.T, opts *testOptions) *testCase {
	if opts == nil {
		opts = &testOptions{}
	}
	if opts.testDir == "" {
		opts.testDir = testutil.TempDir()
	}
	tc := &testCase{
		dir: opts.testDir,
		T:   t,
	}
	tc.origDir = tc.dir + "/orig"
	tc.mntDir = tc.dir + "/mnt"
	if err := os.Mkdir(tc.origDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tc.mntDir, 0755); err != nil {
		t.Fatal(err)
	}

	var err error
	tc.loopback, err = NewLoopbackRoot(tc.origDir)
	if err != nil {
		t.Fatalf("NewLoopback: %v", err)
	}

	oneSec := time.Second

	attrDT := &oneSec
	if !opts.attrCache {
		attrDT = nil
	}
	entryDT := &oneSec
	if !opts.entryCache {
		entryDT = nil
	}
	tc.rawFS = NewNodeFS(tc.loopback, &Options{
		EntryTimeout: entryDT,
		AttrTimeout:  attrDT,
	})

	mOpts := &fuse.MountOptions{}
	if !opts.suppressDebug {
		mOpts.Debug = testutil.VerboseTest()
	}
	if opts.ro {
		mOpts.Options = append(mOpts.Options, "ro")
	}
	tc.server, err = fuse.NewServer(tc.rawFS, tc.mntDir, mOpts)
	if err != nil {
		t.Fatal(err)
	}

	go tc.server.Serve()
	if err := tc.server.WaitMount(); err != nil {
		t.Fatal(err)
	}
	return tc
}

func TestBasic(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

	tc.writeOrig("file", "hello", 0644)

	fn := tc.mntDir + "/file"
	fi, err := os.Lstat(fn)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if fi.Size() != 5 {
		t.Errorf("got size %d want 5", fi.Size())
	}

	stat := fuse.ToStatT(fi)
	if got, want := uint32(stat.Mode), uint32(fuse.S_IFREG|0644); got != want {
		t.Errorf("got mode %o, want %o", got, want)
	}

	if err := os.Remove(fn); err != nil {
		t.Errorf("Remove: %v", err)
	}

	if fi, err := os.Lstat(fn); err == nil {
		t.Errorf("Lstat after remove: got file %v", fi)
	}
}

func TestFileFdLeak(t *testing.T) {
	tc := newTestCase(t, &testOptions{
		suppressDebug: true,
		attrCache:     true,
		entryCache:    true,
	})
	defer func() {
		if tc != nil {
			tc.Clean()
		}
	}()

	posixtest.FdLeak(t, tc.mntDir)

	tc.Clean()
	bridge := tc.rawFS.(*rawBridge)
	tc = nil

	if got := len(bridge.files); got > 3 {
		t.Errorf("found %d used file handles, should be <= 3", got)
	}
}

func TestNotifyEntry(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

	orig := tc.origDir + "/file"
	fn := tc.mntDir + "/file"
	tc.writeOrig("file", "hello", 0644)

	st := syscall.Stat_t{}
	if err := syscall.Lstat(fn, &st); err != nil {
		t.Fatalf("Lstat before: %v", err)
	}

	if err := os.Remove(orig); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	after := syscall.Stat_t{}
	if err := syscall.Lstat(fn, &after); err != nil {
		t.Fatalf("Lstat after: %v", err)
	} else if !reflect.DeepEqual(st, after) {
		t.Fatalf("got after %#v, want %#v", after, st)
	}

	if errno := tc.loopback.EmbeddedInode().NotifyEntry("file"); errno != 0 {
		t.Errorf("notify failed: %v", errno)
	}

	if err := syscall.Lstat(fn, &after); err != syscall.ENOENT {
		t.Fatalf("Lstat after: got %v, want ENOENT", err)
	}
}

func TestReadDirStress(t *testing.T) {
	tc := newTestCase(t, &testOptions{suppressDebug: true, attrCache: true, entryCache: true})
	defer tc.Clean()

	// Create 110 entries
	for i := 0; i < 110; i++ {
		name := fmt.Sprintf("file%036x", i)
		if err := ioutil.WriteFile(filepath.Join(tc.mntDir, name), []byte("hello"), 0644); err != nil {
			t.Fatalf("WriteFile %q: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	stress := func(gr int) {
		defer wg.Done()
		for i := 1; i < 100; i++ {
			f, err := os.Open(tc.mntDir)
			if err != nil {
				t.Error(err)
				return
			}
			_, err = f.Readdirnames(-1)
			if err != nil {
				t.Errorf("goroutine %d iteration %d: %v", gr, i, err)
				f.Close()
				return
			}
			f.Close()
		}
	}

	n := 3
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go stress(i)
	}
	wg.Wait()
}

// This test is racy. If an external process consumes space while this
// runs, we may see spurious differences between the two statfs() calls.
func TestStatFs(t *testing.T) {
	tc := newTestCase(t, &testOptions{attrCache: true, entryCache: true})
	defer tc.Clean()

	empty := syscall.Statfs_t{}
	orig := empty
	if err := syscall.Statfs(tc.origDir, &orig); err != nil {
		t.Fatal("statfs orig", err)
	}

	mnt := syscall.Statfs_t{}
	if err := syscall.Statfs(tc.mntDir, &mnt); err != nil {
		t.Fatal("statfs mnt", err)
	}

	var mntFuse, origFuse fuse.StatfsOut
	mntFuse.FromStatfsT(&mnt)
	origFuse.FromStatfsT(&orig)

	if !reflect.DeepEqual(mntFuse, origFuse) {
		t.Errorf("Got %#v, want %#v", mntFuse, origFuse)
	}
}

func TestGetAttrParallel(t *testing.T) {
	// We grab a file-handle to provide to the API so rename+fstat
	// can be handled correctly. Here, test that closing and
	// (f)stat in parallel don't lead to fstat on closed files.
	// We can only test that if we switch off caching
	tc := newTestCase(t, &testOptions{suppressDebug: true})
	defer tc.Clean()

	N := 100

	var fds []int
	var fns []string
	for i := 0; i < N; i++ {
		fn := fmt.Sprintf("file%d", i)
		tc.writeOrig(fn, "ello", 0644)
		fn = filepath.Join(tc.mntDir, fn)
		fns = append(fns, fn)
		fd, err := syscall.Open(fn, syscall.O_RDONLY, 0)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}

		fds = append(fds, fd)
	}

	var wg sync.WaitGroup
	wg.Add(2 * N)
	for i := 0; i < N; i++ {
		go func(i int) {
			if err := syscall.Close(fds[i]); err != nil {
				t.Errorf("close %d: %v", i, err)
			}
			wg.Done()
		}(i)
		go func(i int) {
			var st syscall.Stat_t
			if err := syscall.Lstat(fns[i], &st); err != nil {
				t.Errorf("lstat %d: %v", i, err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestMknod(t *testing.T) {
	tc := newTestCase(t, &testOptions{})
	defer tc.Clean()

	modes := map[string]uint32{
		"regular": syscall.S_IFREG,
		"socket":  syscall.S_IFSOCK,
		"fifo":    syscall.S_IFIFO,
	}

	for nm, mode := range modes {
		t.Run(nm, func(t *testing.T) {
			p := filepath.Join(tc.mntDir, nm)
			err := syscall.Mknod(p, mode|0755, (8<<8)|0)
			if err != nil {
				t.Fatalf("mknod(%s): %v", nm, err)
			}

			var st syscall.Stat_t
			if err := syscall.Stat(p, &st); err != nil {
				got := st.Mode &^ 07777
				if want := mode; got != want {
					t.Fatalf("stat(%s): got %o want %o", nm, got, want)
				}
			}

			// We could test if the files can be
			// read/written but: The kernel handles FIFOs
			// without talking to FUSE at all. Presumably,
			// this also holds for sockets.  Regular files
			// are tested extensively elsewhere.
		})
	}
}

func TestPosix(t *testing.T) {
	noisy := map[string]bool{
		"ParallelFileOpen": true,
		"ReadDir":          true,
	}

	for nm, fn := range posixtest.All {
		t.Run(nm, func(t *testing.T) {
			tc := newTestCase(t, &testOptions{
				suppressDebug: noisy[nm],
				attrCache:     true, entryCache: true})
			defer tc.Clean()

			fn(t, tc.mntDir)
		})
	}
}

func TestOpenDirectIO(t *testing.T) {
	// Apparently, tmpfs does not allow O_DIRECT, so try to create
	// a test temp directory in /var/tmp.
	ext4Dir, err := ioutil.TempDir("/var/tmp", "go-fuse.TestOpenDirectIO")
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	defer os.RemoveAll(ext4Dir)

	posixtest.DirectIO(t, ext4Dir)
	if t.Failed() {
		t.Skip("DirectIO failed on underlying FS")
	}

	opts := testOptions{
		testDir:    ext4Dir,
		attrCache:  true,
		entryCache: true,
	}

	tc := newTestCase(t, &opts)
	defer tc.Clean()
	posixtest.DirectIO(t, tc.mntDir)
}

// TestFsstress is loosely modeled after xfstest's fsstress. It performs rapid
// parallel removes / creates / readdirs. Coupled with inode reuse, this test
// used to deadlock go-fuse quite quickly.
func TestFsstress(t *testing.T) {
	tc := newTestCase(t, &testOptions{suppressDebug: true, attrCache: true, entryCache: true})
	defer tc.Clean()

	{
		old := runtime.GOMAXPROCS(100)
		defer runtime.GOMAXPROCS(old)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	ops := map[string]func(string) error{
		"mkdir":      func(p string) error { return syscall.Mkdir(p, 0700) },
		"mknod_reg":  func(p string) error { return syscall.Mknod(p, 0700|syscall.S_IFREG, 0) },
		"remove":     os.Remove,
		"unlink":     syscall.Unlink,
		"mknod_sock": func(p string) error { return syscall.Mknod(p, 0700|syscall.S_IFSOCK, 0) },
		"mknod_fifo": func(p string) error { return syscall.Mknod(p, 0700|syscall.S_IFIFO, 0) },
		"mkfifo":     func(p string) error { return syscall.Mkfifo(p, 0700) },
		"symlink":    func(p string) error { return syscall.Symlink("foo", p) },
		"creat": func(p string) error {
			fd, err := syscall.Open(p, syscall.O_CREAT|syscall.O_EXCL, 0700)
			if err == nil {
				syscall.Close(fd)
			}
			return err
		},
	}

	opLoop := func(k string, n int) {
		defer wg.Done()
		op := ops[k]
		for {
			p := fmt.Sprintf("%s/%s.%d", tc.mntDir, t.Name(), n)
			op(p)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	readdirLoop := func() {
		defer wg.Done()
		for {
			f, err := os.Open(tc.mntDir)
			if err != nil {
				panic(err)
			}
			_, err = f.Readdir(0)
			if err != nil {
				panic(err)
			}
			f.Close()
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	for i := 1; i < 10; i++ {
		for k := range ops {
			wg.Add(1)
			go opLoop(k, i)
		}
	}

	wg.Add(1)
	go readdirLoop()

	// An external "ls" loop has a destructive effect that I am unable to
	// reproduce through in-process operations.
	if strings.ContainsAny(tc.mntDir, "'\\") {
		// But let's not enable shell injection.
		log.Panicf("shell injection attempt? mntDir=%q", tc.mntDir)
	}
	// --color=always enables xattr lookups for extra stress
	cmd := exec.Command("bash", "-c", "while true ; do ls -l --color=always '"+tc.mntDir+"'; done")
	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	// Run the test for 10 seconds
	time.Sleep(10 * time.Second)

	cancel()

	wg.Wait()
}

func init() {
	syscall.Umask(0)
}
