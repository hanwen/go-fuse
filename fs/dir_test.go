// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type dirStreamErrorNode struct {
	Inode
}

var _ = (NodeReaddirer)((*dirStreamErrorNode)(nil))

func (n *dirStreamErrorNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	return &errDirStream{}, 0
}

type errDirStream struct {
	num int
}

func (ds *errDirStream) HasNext() bool {
	return ds.num < 2
}

func (ds *errDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	ds.num++
	if ds.num == 1 {
		return fuse.DirEntry{
			Mode: fuse.S_IFREG,
			Name: "first",
			Ino:  2,
			Off:  100,
		}, 0
	}
	if ds.num == 2 {
		return fuse.DirEntry{
			Mode: fuse.S_IFREG,
			Name: "last",
			Ino:  3,
			Off:  200,
		}, syscall.EBADMSG
	}

	panic("boom")
}

func (ds *errDirStream) Close() {

}

func TestDirStreamError(t *testing.T) {
	for _, disableReaddirplus := range []bool{false, true} {
		t.Run(fmt.Sprintf("disableReaddirplus=%v", disableReaddirplus),
			func(t *testing.T) {
				root := &dirStreamErrorNode{}
				opts := Options{}
				opts.DisableReadDirPlus = disableReaddirplus

				mnt, _ := testMount(t, root, &opts)

				ds, errno := NewLoopbackDirStream(mnt)
				if errno != 0 {
					t.Fatalf("NewLoopbackDirStream: %v", errno)
				}
				defer ds.Close()

				if !ds.HasNext() {
					t.Fatal("expect HasNext")
				}
				if e, errno := ds.Next(); errno != 0 {
					t.Errorf("ds.Next: %v", errno)
				} else if e.Name != "first" {
					t.Errorf("got %q want 'first'", e.Name)
				} else if e.Off != 100 {
					t.Errorf("got off %d, want 100", e.Off)
				}

				if !ds.HasNext() {
					t.Fatalf("got !HasNext")
				}

				// Here we need choose a errno to test if errno could be passed and handled
				// correctly by the fuse library. To build the test on different platform,
				// an errno which defined on each platform should be chosen. And if the
				// chosen integer number is not a valid errno, the fuse in kernel would refuse
				// and throw an error, which is observed on Linux.
				// Here we choose to use EBADMSG, which is defined on multiple Unix-like OSes.
				if _, errno := ds.Next(); errno != syscall.EBADMSG {
					t.Errorf("got errno %v, want EBADMSG", errno)
				}
			})
	}
}

type dirStreamSeekNode struct {
	Inode
	num int
}

type listDirEntries struct {
	entries []fuse.DirEntry
	next    int
}

var _ = (FileReaddirenter)((*listDirEntries)(nil))

func (l *listDirEntries) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if l.next >= len(l.entries) {
		return nil, 0
	}
	de := &l.entries[l.next]
	l.next++
	return de, 0
}

var _ = (FileSeekdirer)((*listDirEntries)(nil))

func (l *listDirEntries) Seekdir(ctx context.Context, off uint64) syscall.Errno {
	if off == 0 {
		l.next = 0
	} else {
		for i, e := range l.entries {
			if e.Off == off {
				l.next = i + 1
				return 0
			}
		}
	}
	// TODO: error code if not found?
	return 0
}

var _ = (NodeOpendirHandler)((*dirStreamSeekNode)(nil))

func (n *dirStreamSeekNode) OpendirHandle(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	var l []fuse.DirEntry

	for i := 0; i < n.num; i++ {
		l = append(l, fuse.DirEntry{
			Name: fmt.Sprintf("name%d", i),
			Mode: fuse.S_IFREG,
			Ino:  uint64(i + 100),
			Off:  uint64((1 + (i*7)%n.num) * 100),
		})
	}

	return &listDirEntries{entries: l}, 0, 0
}

func testDirSeek(t *testing.T, mnt string) {
	ds, errno := NewLoopbackDirStream(mnt)
	if errno != 0 {
		t.Fatalf("NewLoopbackDirStream: %v", errno)
	}
	defer ds.Close()

	fullResult, errno := readDirStream(ds)
	if errno != 0 {
		t.Fatalf("readDirStream: %v", errno)
	}

	for i, res := range fullResult {
		func() {
			ds, errno := NewLoopbackDirStream(mnt)
			if errno != 0 {
				t.Fatalf("NewLoopbackDirStream: %v", errno)
			}
			defer ds.Close()

			if errno := ds.(*loopbackDirStream).Seekdir(context.Background(), res.Off); errno != 0 {
				t.Fatalf("seek: %v", errno)
			}

			rest, errno := readDirStream(ds)
			if errno != 0 {
				t.Fatalf("readDirStream: %v", errno)
			}
			if rest == nil {
				rest = fullResult[:0]
			}
			if want := fullResult[i+1:]; !reflect.DeepEqual(rest, want) {
				t.Errorf("got %v, want %v", rest, want)
			}
		}()
	}
}

func TestDirStreamSeek(t *testing.T) {
	for _, rdp := range []bool{false, true} {
		t.Run(fmt.Sprintf("readdirplus=%v", rdp),
			func(t *testing.T) {
				N := 11

				root := &dirStreamSeekNode{num: N}
				opts := Options{}
				opts.DisableReadDirPlus = !rdp

				mnt, _ := testMount(t, root, &opts)
				testDirSeek(t, mnt)
			})
	}
}

type syncNode struct {
	Inode

	mu           sync.Mutex
	syncDirCount int
}

type syncDir struct {
	node *syncNode
}

func (d *syncDir) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	return nil, 0
}

var _ = (FileFsyncdirer)((*syncDir)(nil))

func (d *syncDir) Fsyncdir(ctx context.Context, flags uint32) syscall.Errno {
	d.node.mu.Lock()
	defer d.node.mu.Unlock()
	d.node.syncDirCount++
	return 0
}

var _ = (NodeOpendirHandler)((*syncNode)(nil))

func (n *syncNode) OpendirHandle(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	return &syncDir{n}, 0, 0
}

func TestFsyncDir(t *testing.T) {
	root := &syncNode{}
	mnt, _ := testMount(t, root, nil)

	fd, err := syscall.Open(mnt, syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Fsync(fd); err != nil {
		t.Fatal(err)
	}
	syscall.Close(fd)
	root.mu.Lock()
	defer root.mu.Unlock()
	if root.syncDirCount != 1 {
		t.Errorf("got %d, want 1", root.syncDirCount)
	}

}
