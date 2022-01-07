// Copyright 2022 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type maxWriteTestRoot struct {
	Inode

	sync.Mutex
	readSize  int
	writeSize int
}

var _ = (NodeOnAdder)((*maxWriteTestRoot)(nil))

func (n *maxWriteTestRoot) OnAdd(ctx context.Context) {
	n.Inode.AddChild("file", n.Inode.NewInode(ctx, &maxWriteTestNode{maxWriteTestRoot: n}, StableAttr{}), false)
}

type maxWriteTestNode struct {
	Inode

	maxWriteTestRoot *maxWriteTestRoot
}

var _ = (NodeGetattrer)((*maxWriteTestNode)(nil))

func (n *maxWriteTestNode) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = 1024 * 1024 * 1024
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
	fh.maxWriteTestRoot.readSize = len(data)
	fh.maxWriteTestRoot.Unlock()
	return fuse.ReadResultData(data), 0
}

var _ = (FileWriter)((*maxWriteTestFH)(nil))

func (fh *maxWriteTestFH) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fh.maxWriteTestRoot.Lock()
	fh.maxWriteTestRoot.writeSize = len(data)
	fh.maxWriteTestRoot.Unlock()
	return uint32(len(data)), 0
}

// TestMaxWrite checks that combinations of the MaxWrite, MaxReadAhead, max_read
// options result in the expected observed read and write sizes from the kernel.
func TestMaxWrite(t *testing.T) {
	type testcase struct {
		// Mount options
		MaxWrite     int
		MaxReadAhead int
		maxRead      int
	}
	testcases := []testcase{
		{
			MaxWrite: 4 * 1024, // lower limit in all Linux versions
		},
		{
			MaxWrite: 8 * 1024,
		},
		{
			MaxWrite: 64 * 1024, // go-fuse default
		},
		{
			MaxWrite: 128 * 1024, // upper limit in Linux v4.19 and older
		},
		{
			MaxWrite:     128 * 1024,
			MaxReadAhead: 4 * 1024,
		},
		{
			MaxWrite: 1024 * 1024, // upper limit in Linux v4.20+
		},
		{
			MaxWrite: 1024 * 1024,
			maxRead:  64 * 1024,
		},
		{
			MaxWrite:     1024 * 1024,
			MaxReadAhead: 16 * 1024,
		},
		{
			MaxWrite:     1024 * 1024,
			MaxReadAhead: 16 * 1024,
			maxRead:      64 * 1024,
		},
		{
			MaxWrite:     1024 * 1024,
			MaxReadAhead: 76 * 1024,
			maxRead:      16 * 1024,
		},
	}

	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	for _, o := range testcases {
		name := fmt.Sprintf("MaxWr%d.MaxRa%d.maxRead%d", o.MaxWrite/1024, o.MaxReadAhead/1024, o.maxRead/1024)
		t.Run(name, func(t *testing.T) {
			// expected results
			directWriteSize := o.MaxWrite
			directReadSize := o.MaxWrite
			if o.maxRead > 0 {
				directReadSize = min(o.MaxWrite, o.maxRead)
			}
			normalWriteSize := o.MaxWrite
			// kernel readahead makes the resulting request sizes unpredictable,
			// but they will be capped at this
			normalReadSizeMax := directReadSize

			mo := fuse.MountOptions{
				MaxWrite:     o.MaxWrite,
				MaxReadAhead: o.MaxReadAhead,
			}
			if o.maxRead != 0 {
				mo.Options = []string{fmt.Sprintf("max_read=%d", o.maxRead)}
			}
			root := &maxWriteTestRoot{
				writeSize: -1,
				readSize:  -1,
			}
			mntDir, srv, clean := testMount(t, root, &Options{MountOptions: mo})
			defer clean()

			if srv.KernelSettings().Flags&fuse.CAP_MAX_PAGES == 0 {
				t.Skip("kernel does not support MAX_PAGES")
			}

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
			_, err = syscall.Pread(fdDirect, buf, 0)
			if err != nil {
				t.Errorf("read failed: %v", err)
			}

			root.Lock()
			if root.readSize != directReadSize {
				t.Errorf("Direct I/O readSize %#v: have=%d, want=%d", o, root.readSize, directReadSize)
			}
			if root.writeSize != directWriteSize {
				t.Errorf("Direct I/O writeSize %#v: have=%d, want=%d", o, root.writeSize, directWriteSize)
			}
			root.writeSize = -1
			root.readSize = -1
			root.Unlock()

			// Normal I/O
			fdNormal, err := syscall.Open(mntDir+"/file", syscall.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.Close(fdNormal)

			_, err = syscall.Pread(fdNormal, buf, 0)
			if err != nil {
				t.Errorf("read failed: %v", err)
			}
			_, err = syscall.Pwrite(fdNormal, buf, 0)
			if err != nil {
				t.Errorf("write failed: %v", err)
			}

			root.Lock()
			if root.readSize > normalReadSizeMax {
				t.Errorf("Normal I/O readSize: have=%d, max=%d", root.readSize, normalReadSizeMax)
			}
			if root.writeSize != normalWriteSize {
				t.Errorf("Normal I/O writeSize: have=%d, want=%d", root.writeSize, normalWriteSize)
			}
			root.Unlock()
		})
	}
}
