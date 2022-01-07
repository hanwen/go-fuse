// Copyright 2022 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type maxWriteTestRoot struct {
	Inode

	sync.Mutex
	// largest observed read size
	largestRead int
	// largest observed write size
	largestWrite int
}

// https://github.com/torvalds/linux/blob/e2ae0d4a6b0ba461542f0fd0ba0b828658013e9f/include/linux/pagemap.h#L999
const VM_READAHEAD = 131072

var _ = (NodeOnAdder)((*maxWriteTestRoot)(nil))

func (n *maxWriteTestRoot) OnAdd(ctx context.Context) {
	n.Inode.AddChild("file", n.Inode.NewInode(ctx, &maxWriteTestNode{maxWriteTestRoot: n}, StableAttr{}), false)
}

func (n *maxWriteTestRoot) resetStats() {
	n.Lock()
	n.largestWrite = 0
	n.largestRead = 0
	n.Unlock()
}

type maxWriteTestNode struct {
	Inode

	maxWriteTestRoot *maxWriteTestRoot
}

var _ = (NodeGetattrer)((*maxWriteTestNode)(nil))

func (n *maxWriteTestNode) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = 1024 * 1024 * 1024 // 1 GiB
	return 0
}

var _ = (NodeOpener)((*maxWriteTestNode)(nil))

func (n *maxWriteTestNode) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return &maxWriteTestFH{n.maxWriteTestRoot}, 0, OK
}

type maxWriteTestFH struct {
	maxWriteTestRoot *maxWriteTestRoot
}

var _ = (FileReader)((*maxWriteTestFH)(nil))

func (fh *maxWriteTestFH) Read(ctx context.Context, data []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fh.maxWriteTestRoot.Lock()
	if fh.maxWriteTestRoot.largestRead < len(data) {
		fh.maxWriteTestRoot.largestRead = len(data)
	}
	fh.maxWriteTestRoot.Unlock()
	return fuse.ReadResultData(data), 0
}

var _ = (FileWriter)((*maxWriteTestFH)(nil))

func (fh *maxWriteTestFH) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fh.maxWriteTestRoot.Lock()
	if fh.maxWriteTestRoot.largestWrite < len(data) {
		fh.maxWriteTestRoot.largestWrite = len(data)
	}
	fh.maxWriteTestRoot.Unlock()
	return uint32(len(data)), 0
}

// TestMaxWrite checks that combinations of the MaxWrite, MaxReadAhead, max_read
// options result in the expected observed read and write sizes from the kernel.
func TestMaxWrite(t *testing.T) {
	testcases := []fuse.MountOptions{
		{
			MaxWrite: 4 * 1024, // 4 kiB (one page) = lower limit in all Linux versions
		},
		{
			MaxWrite: 8 * 1024,
		},
		{
			MaxWrite: 9999, // let's see what happens if this is unaligned
		},
		{
			MaxWrite: 64 * 1024, // 64 kiB = go-fuse default
		},
		{
			MaxWrite: 128 * 1024, // 128 kiB = upper limit in Linux v4.19 and older
		},
		{
			MaxWrite: 1024 * 1024, // 1 MiB = upper limit in Linux v4.20+
		},
		// cycle through readahead values
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 4 * 1024,
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 8 * 1024,
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 16 * 1024,
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 32 * 1024,
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 64 * 1024,
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 128 * 1024,
		},
		{
			// both at default
		},
		{
			// default MaxWrite
			MaxReadAhead: 4 * 1024,
		},
	}

	for _, tc := range testcases {
		name := fmt.Sprintf("MaxWr%d.MaxRa%d", tc.MaxWrite, tc.MaxReadAhead)
		t.Run(name, func(t *testing.T) {
			root := &maxWriteTestRoot{}
			root.resetStats()

			mntDir, srv, clean := testMount(t, root, &Options{MountOptions: tc})
			defer clean()

			readAheadWant := tc.MaxReadAhead
			if readAheadWant == 0 {
				readAheadWant = VM_READAHEAD
			}
			readAheadHave := bdiReadahead(mntDir)
			if readAheadHave != readAheadWant {
				t.Errorf("Readahead mismatch: have=bdiReadahead=%d want=%d", readAheadHave, readAheadWant)
			}

			actualMaxWrite := tc.MaxWrite
			if srv.KernelSettings().Flags&fuse.CAP_MAX_PAGES == 0 && actualMaxWrite > 128*1024 {
				// Kernel 4.19 and lower don't have CAP_MAX_PAGES and limit to 128 kiB.
				actualMaxWrite = 128 * 1024
			} else if tc.MaxWrite == 0 {
				actualMaxWrite = 128 * 1024
			}

			// Try to make 2 MiB requests, which is more than the kernel supports, so
			// we will observe the imposed limits in the actual request sizes.
			buf := make([]byte, 2*1024*1024)

			// Direct I/O
			fdDirect, err := syscall.Open(mntDir+"/file", syscall.O_RDWR|syscall.O_DIRECT, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.Close(fdDirect)

			_, err = syscall.Pwrite(fdDirect, buf, 0)
			if err != nil {
				t.Errorf("write failed: %v", err)
			}
			root.Lock()
			if root.largestWrite != actualMaxWrite {
				t.Errorf("Direct I/O largestWrite: have=%d, want=%d", root.largestWrite, actualMaxWrite)
			}
			root.Unlock()

			_, err = syscall.Pread(fdDirect, buf, 0)
			if err != nil {
				t.Errorf("read failed: %v", err)
			}
			root.Lock()
			if root.largestRead != actualMaxWrite {
				t.Errorf("Direct I/O largestRead: have=%d, want=%d", root.largestRead, actualMaxWrite)
			}
			root.Unlock()

			root.resetStats()

			// Buffered I/O
			fdBuffered, err := syscall.Open(mntDir+"/file", syscall.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.Close(fdBuffered)

			// Buffered read
			_, err = syscall.Pread(fdBuffered, buf, 0)
			if err != nil {
				t.Errorf("read failed: %v", err)
			}
			root.Lock()
			// On Linux 4.19, I get exactly tc.MaxReadAhead, while on 6.0 I also get
			// larger reads up to 128 kiB. We log the results but don't expect anything.
			t.Logf("Buffered I/O largestRead: have=%d", root.largestRead)
			root.Unlock()

			// Buffered write
			_, err = syscall.Pwrite(fdBuffered, buf, 0)
			if err != nil {
				t.Errorf("write failed: %v", err)
			}
			root.Lock()
			if root.largestWrite != actualMaxWrite {
				t.Errorf("Buffered I/O largestWrite: have=%d, want=%d", root.largestWrite, actualMaxWrite)
			}
			root.Unlock()
		})
	}
}

// bdiReadahead extracts the readahead size (in bytes) of the filesystem at mnt from
// /sys/class/bdi/%d:%d/read_ahead_kb .
func bdiReadahead(mnt string) int {
	var st syscall.Stat_t
	err := syscall.Stat(mnt, &st)
	if err != nil {
		panic(err)
	}
	path := fmt.Sprintf("/sys/class/bdi/%d:%d/read_ahead_kb", unix.Major(st.Dev), unix.Minor(st.Dev))
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	trimmed := strings.TrimSpace(string(buf))
	val, err := strconv.Atoi(trimmed)
	if err != nil {
		panic(err)
	}
	return val * 1024
}
