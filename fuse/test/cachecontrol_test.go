// Copyright 2018 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

// exercise functionality to store/retrieve kernel cache.

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// DataNode is a nodefs.Node that Reads static data.
//
// Since Read works without Open, the kernel does not invalidate DataNode cache
// on user's open.
type DataNode struct {
	nodefs.Node

	data []byte
}

func NewDataNode(data []byte) *DataNode {
	return &DataNode{Node: nodefs.NewDefaultNode(), data: data}
}

func (d *DataNode) GetAttr(out *fuse.Attr, _ nodefs.File, _ *fuse.Context) fuse.Status {
	out.Size = uint64(len(d.data))
	out.Mode = fuse.S_IFREG | 0644
	return fuse.OK
}

func (d *DataNode) Read(_ nodefs.File, dest []byte, off int64, _ *fuse.Context) (fuse.ReadResult, fuse.Status) {
	l := int64(len(d.data))
	end := off + l
	if end > l {
		end = l
	}

	return fuse.ReadResultData(d.data[off:end]), fuse.OK
}

// TestCacheControl verifies that FUSE server process can store/retrieve kernel data cache.
func TestCacheControl(t *testing.T) {
	dir := testutil.TempDir()
	defer func() {
		err := os.Remove(dir)
		if err != nil {
			t.Fatal(err)
		}
	}()

	// setup a filesystem with 1 file
	root := nodefs.NewDefaultNode()
	opts := nodefs.NewOptions()
	opts.Debug = testutil.VerboseTest()
	srv, fsconn, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatal(err)
	}

	data0 := "hello world"
	file := NewDataNode([]byte(data0))
	root.Inode().NewChild("hello.txt", false, file)

	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal("WaitMount", err)
	}
	defer func() {
		err := srv.Unmount()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// assertFileRead asserts that the file content reads as dataOK.
	assertFileRead := func(subj, dataOK string) {
		t.Helper()

		v, err := ioutil.ReadFile(dir + "/hello.txt")
		if err != nil {
			t.Fatalf("%s: file read: %s", subj, err)
		}
		if string(v) != dataOK {
			t.Fatalf("%s: file read: got %q  ; want %q", subj, v, dataOK)
		}
	}

	// assertCacheRead asserts that file's kernel cache is retrieved as dataOK.
	assertCacheRead := func(subj, dataOK string) {
		t.Helper()

		assertCacheReadAt := func(offset int64, size int, dataOK string) {
			t.Helper()
			buf := make([]byte, size)
			n, st := fsconn.FileRetrieveCache(file.Inode(), offset, buf)
			if st != fuse.OK {
				t.Fatalf("%s: retrieve cache @%d [%d]: %s", subj, offset, size, st)
			}
			if got := buf[:n]; string(got) != dataOK {
				t.Fatalf("%s: retrieve cache @%d [%d]: have %q; want %q", subj, offset, size, got, dataOK)
			}
		}

		// retrieve [1:len - 1]   (also verifying that offset/size are handled correctly)
		l := len(dataOK)
		if l >= 2 {
			assertCacheReadAt(1, l-2, dataOK[1:l-1])
		}

		// retrieve [:∞]
		assertCacheReadAt(0, l+10000, dataOK)
	}

	// x<f> is <f> that t.Fatals on error
	xFileNotifyStoreCache := func(subj string, inode *nodefs.Inode, offset int64, data []byte) {
		t.Helper()
		st := fsconn.FileNotifyStoreCache(inode, offset, data)
		if st != fuse.OK {
			t.Fatalf("store cache (%s): %s", subj, st)
		}
	}

	xmmap := func(fd int, offset int64, size, prot, flags int) []byte {
		t.Helper()
		mem, err := unix.Mmap(fd, offset, size, prot, flags)
		if err != nil {
			t.Fatal(err)
		}
		return mem
	}

	xmunmap := func(mem []byte) {
		t.Helper()
		err := unix.Munmap(mem)
		if err != nil {
			t.Fatal(err)
		}
	}

	xmlock := func(mem []byte) {
		t.Helper()
		err := unix.Mlock(mem)
		if err != nil {
			t.Fatal(err)
		}
	}

	// before the kernel has entry for file in its dentry cache, the cache
	// should read as empty and cache store should fail with ENOENT.
	assertCacheRead("before lookup", "")
	st := fsconn.FileNotifyStoreCache(file.Inode(), 0, []byte("abc"))
	if st != fuse.ENOENT {
		t.Fatalf("%s: store cache -> %v; want %v", "before lookup", st, fuse.ENOENT)
	}

	// lookup on the file - forces to assign inode ID to it
	os.Stat(dir + "/hello.txt")

	// cache should be initially empty
	assertCacheRead("initial", "")

	// make sure the file reads correctly
	assertFileRead("original", data0)

	// pin file content into OS cache
	f, err := os.Open(dir + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	fmmap := xmmap(int(f.Fd()), 0, len(data0), unix.PROT_READ, unix.MAP_SHARED)
	defer func() {
		err := f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer func() {
		xmunmap(fmmap)
	}()

	// assertMmapRead asserts that file's mmaped memory reads as dataOK.
	assertMmapRead := func(subj, dataOK string) {
		t.Helper()
		// Use the Mlock() syscall to get the mmap'ed range into the kernel
		// cache again, triggering FUSE reads as neccessary. A blocked syscall does
		// not count towards GOMAXPROCS, so there should be a thread available
		// to handle the FUSE reads.
		// If we don't Mlock() first, the memory comparison triggers a page
		// fault, which blocks the thread, and deadlocks the test reliably at
		// GOMAXPROCS=1.
		// Fixes https://github.com/hanwen/go-fuse/issues/261 .
		xmlock(fmmap)
		if string(fmmap) != dataOK {
			t.Fatalf("%s: file mmap: got %q  ; want %q", subj, fmmap, dataOK)
		}
	}

	// make sure the cache has original data
	assertMmapRead("original", data0)
	assertCacheRead("original", data0)

	// store changed data into OS cache
	xFileNotifyStoreCache("", file.Inode(), 7, []byte("123"))

	// make sure mmaped data and file read as updated data
	data1 := "hello w123d"
	assertMmapRead("after storecache", data1)
	assertFileRead("after storecache", data1)
	assertCacheRead("after storecache", data1)

	// invalidate cache
	st = fsconn.FileNotify(file.Inode(), 0, 0)
	if st != fuse.OK {
		t.Fatalf("invalidate cache: %s", st)
	}

	// make sure cache reads as empty right after invalidation
	assertCacheRead("after invalcache", "")

	// make sure mmapped data, file and cache read as original data
	assertMmapRead("after invalcache", data0)
	assertFileRead("after invalcache", data0)
	assertCacheRead("after invalcache + refill", data0)

	// make sure we can store/retrieve into/from OS cache big chunk of data.
	// we will need to mlock(1M), so skip this test if mlock limit is tight.
	const lbig = 1024 * 1024 // 1M
	const memlockNeed = lbig + /* we already used some small mlock above */ 64*1024
	var lmemlock unix.Rlimit
	err = unix.Getrlimit(unix.RLIMIT_MEMLOCK, &lmemlock)
	if err != nil {
		t.Fatal(err)
	}
	t0 := t
	tp := &t
	t.Run("BigChunk", func(t *testing.T) {
		if lmemlock.Cur < memlockNeed {
			t.Skipf("skipping: too low memlock limit: %dK, need at least %dK", lmemlock.Cur/1024, memlockNeed/1024)
		}

		*tp = t // so that x<f> defined above work with local - not parent - t
		defer func() {
			*tp = t0
		}()

		// big chunk of data to be stored into OS cache, enlarging the file there.
		//
		// The values are unique uint32, so that if the kernel or our glue in
		// InodeRetrieveCache makes a mistake, we should notice by seeing
		// different data.
		//
		// Use 0x01020304 as of base offset so that there are no e.g. '00 00 00 01
		// 00 00 00 02 ...' sequence - i.e. where we could not notice if e.g. a
		// single-byte offset bug is there somewhere.
		buf := &bytes.Buffer{}
		for i := uint32(0); i < lbig/4; i++ {
			err := binary.Write(buf, binary.BigEndian, i+0x01020304)
			if err != nil {
				panic(err) // Buffer.Write does not error
			}
		}
		dataBig := buf.String()

		// first store zeros and mlock, so that OS does not loose the cache
		xFileNotifyStoreCache("big, 0", file.Inode(), 0, make([]byte, lbig))
		fmmapBig := xmmap(int(f.Fd()), 0, lbig, unix.PROT_READ, unix.MAP_SHARED)
		defer xmunmap(fmmapBig)
		xmlock(fmmapBig)

		// upload big data into pinned cache pages
		xFileNotifyStoreCache("big", file.Inode(), 0, []byte(dataBig))

		// make sure we can retrieve a big chunk from the cache
		assertCacheRead("after storecache big", dataBig)
	})
}
