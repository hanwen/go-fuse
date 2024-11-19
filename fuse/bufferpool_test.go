// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"os"
	"testing"

	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

func TestBufferPool(t *testing.T) {
	bp := bufferPool{}
	size := 1500
	buf1 := bp.AllocBuffer(uint32(size))
	if len(buf1) != size {
		t.Errorf("Expected buffer of %d bytes, got %d bytes", size, len(buf1))
	}
	bp.FreeBuffer(buf1)

	// tried testing to see if we get buf1 back if we ask again,
	// but it's not guaranteed and sometimes fails
}

type readFS struct {
	defaultRawFileSystem
}

func (fs *readFS) Open(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status) {
	if input.NodeId != 2 {
		return ENOENT
	}

	return OK
}

func (fs *readFS) Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status) {
	if input.NodeId != 2 {
		return nil, ENOENT
	}

	dest := buf[:input.Size]
	for i := range dest {
		dest[i] = 'x'
	}

	return ReadResultData(dest), OK
}

func (f *readFS) Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) (code Status) {
	if name != "file" {
		return ENOENT
	}

	*out = EntryOut{
		NodeId: 2,
		Attr: Attr{
			Mode: S_IFREG | 0666,
			Size: 1 << 20,
		},
		AttrValid: 1,
	}

	return OK
}

func TestBufferPoolRequestHandler(t *testing.T) {
	mnt := t.TempDir()
	opts := &MountOptions{
		Debug: testutil.VerboseTest(),
	}

	rfs := readFS{}
	srv, err := NewServer(&rfs, mnt, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.ReadFile(mnt + "/file"); err != nil {
		t.Fatal(err)
	}

	ctr := srv.buffers.counters()
	for i, c := range ctr {
		if c != 0 {
			t.Errorf("page count %d: %d buffers outstanding", i, c)
		}
	}

	// TODO: test write as well?
}
