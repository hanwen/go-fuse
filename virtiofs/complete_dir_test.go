// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package virtiofs

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// completeDirFS counts lookups by name and marks all directories with
// FOPEN_COMPLETE_DIR.
type completeDirFS struct {
	fuse.RawFileSystem

	mu      sync.Mutex
	lookups map[string]int
}

func (c *completeDirFS) Lookup(ctx context.Context, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	c.mu.Lock()
	c.lookups[name]++
	c.mu.Unlock()
	return c.RawFileSystem.Lookup(ctx, header, name, out)
}

func (c *completeDirFS) OpenDir(ctx context.Context, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	st := c.RawFileSystem.OpenDir(ctx, input, out)
	if st.Ok() {
		out.OpenFlags |= fuse.FOPEN_COMPLETE_DIR
	}
	return st
}

func (c *completeDirFS) lookupCount(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k, v := range c.lookups {
		if strings.HasPrefix(k, prefix) {
			n += v
		}
	}
	return n
}

// TestCompleteDir checks that after a full READDIRPLUS of a directory
// opened with FOPEN_COMPLETE_DIR, lookups of absent names are answered by
// the guest kernel, also after local creates, and that names in the
// listing and names created or renamed afterwards still resolve once
// their dentries are dropped.
func TestCompleteDir(t *testing.T) {
	orig := t.TempDir()
	const nfiles = 500
	for _, d := range []string{"dir", "empty"} {
		if err := os.Mkdir(orig+"/"+d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Enough entries to need several READDIRPLUS requests.
	for i := 0; i < nfiles; i++ {
		if err := os.WriteFile(fmt.Sprintf("%s/dir/f-%04d", orig, i), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	root, err := fs.NewLoopbackRoot(orig)
	if err != nil {
		t.Fatal(err)
	}
	timeout := time.Minute
	opts := &fs.Options{
		EntryTimeout: &timeout,
		AttrTimeout:  &timeout,
	}
	opts.Debug = testutil.VerboseTest()
	opts.MountOptions.ExtraCapabilities = fuse.CAP_HAS_COMPLETE_DIR

	r := &killNotifyRoot{
		LoopbackNode: root.(*fs.LoopbackNode),
	}
	r.notify = sync.NewCond(&r.mu)
	cfs := &completeDirFS{
		RawFileSystem: fs.NewNodeFS(r, opts),
		lookups:       map[string]int{},
	}

	tmpDir := t.TempDir()
	sockpath := tmpDir + "/virtiofs.socket"
	ramdisk := tmpDir + "/initrd.cpio.gz"

	ps := fuse.NewProtocolServer(cfs, &opts.MountOptions)
	go serveProtocol(sockpath, ps)

	initScript := "#!/bin/sh\n" +
		"mount -t proc none /proc\n" +
		"mount -t sysfs none /sys\n" +
		"mkdir -p /dev && mount -t devtmpfs none /dev\n" +
		moduleInsmodLines(testAssets.modules) +
		`
set -x
mount -t virtiofs myfs /mnt

check() {
  stat "$1" > /dev/null 2>&1
  echo "$1 $?" >> /mnt/result.txt
}

# Keep the directory inode alive across drop_caches.
cd /mnt/dir
echo "count $(ls -1 | wc -l)" >> /mnt/result.txt

for i in $(seq 1 50); do check missing-$i; done

# The listing of an empty directory is valid through the entry timeout of ".".
ls -1 /mnt/empty
for i in $(seq 1 5); do check /mnt/empty/missing-empty-$i; done

echo 2 > /proc/sys/vm/drop_caches
check f-0000
check f-0250
check f-0499
check missing-after-drop

touch new
mv f-0001 g-0001
rm f-0002
echo 2 > /proc/sys/vm/drop_caches
check new
check g-0001
check f-0002
check missing-after-create

cd /
ls /mnt/killme.txt
reboot -n -f
`
	if err := mkinitRam(ramdisk, testAssets.busybox, testAssets.modules, []byte(initScript), nil); err != nil {
		t.Fatal(err)
	}

	cmd := newQemuCmd(sockpath, ramdisk)
	log.Println("running", cmd.Args)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		r.mu.Lock()
		for !r.seenKill {
			r.notify.Wait()
		}
		r.mu.Unlock()
		cmd.Process.Kill()
	}()
	cmd.Wait()

	result, err := os.ReadFile(orig + "/result.txt")
	if err != nil {
		t.Fatalf("guest did not write result.txt: %v", err)
	}
	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(result)), "\n") {
		k, v, _ := strings.Cut(l, " ")
		got[k] = v
	}
	want := map[string]string{
		"count":                fmt.Sprint(nfiles),
		"f-0000":               "0",
		"f-0250":               "0",
		"f-0499":               "0",
		"missing-after-drop":   "1",
		"new":                  "0",
		"g-0001":               "0",
		"f-0002":               "1",
		"missing-after-create": "1",
	}
	for i := 1; i <= 50; i++ {
		want[fmt.Sprintf("missing-%d", i)] = "1"
	}
	for i := 1; i <= 5; i++ {
		want[fmt.Sprintf("/mnt/empty/missing-empty-%d", i)] = "1"
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}

	if ps.KernelSettings().Flags64()&fuse.CAP_HAS_COMPLETE_DIR == 0 {
		t.Skip("guest kernel does not support CAP_HAS_COMPLETE_DIR")
	}

	// The kernel uses a Bloom filter with about 0.1% false
	// positives at this size.
	if n := cfs.lookupCount("missing-"); n > 2 {
		t.Errorf("got %d LOOKUPs for absent names, want at most 2", n)
	}
	// The filter of an empty directory has no bits set.
	if n := cfs.lookupCount("missing-empty-"); n != 0 {
		t.Errorf("got %d LOOKUPs in the empty directory, want 0", n)
	}
	if n := cfs.lookupCount("f-"); n == 0 {
		t.Errorf("no LOOKUPs for listed names after drop_caches; dentries were not dropped")
	}
}
