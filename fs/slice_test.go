// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"bytes"
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// vectorFile is a regular file whose Read returns ReadResultVector,
// exercising the withSlice / writev(2) code path through a real mount.
type vectorFile struct {
	Inode
	vecs [][]byte
}

var _ NodeOpener = (*vectorFile)(nil)
var _ NodeReader = (*vectorFile)(nil)
var _ NodeGetattrer = (*vectorFile)(nil)

func (f *vectorFile) Open(context.Context, uint32) (FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, OK
}

func (f *vectorFile) Getattr(_ context.Context, _ FileHandle, out *fuse.AttrOut) syscall.Errno {
	n := 0
	for _, v := range f.vecs {
		n += len(v)
	}
	out.Size = uint64(n)
	return OK
}

func (f *vectorFile) Read(_ context.Context, _ FileHandle, _ []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off > 0 {
		// Kernels without a page cache, like OpenBSD's, read until
		// they get no data.
		return fuse.ReadResultData(nil), OK
	}
	return fuse.ReadResultVector(f.vecs), OK
}

// TestReadResultVector verifies that a file returning ReadResultVector
// is served correctly through a real FUSE mount (writev path).
func TestReadResultVector(t *testing.T) {
	tests := []struct {
		name string
		vecs [][]byte
		want []byte
	}{
		{"two slices", [][]byte{[]byte("hello, "), []byte("world!")}, []byte("hello, world!")},
		{"single slice", [][]byte{[]byte("only")}, []byte("only")},
		{"three slices", [][]byte{[]byte("a"), []byte("b"), []byte("c")}, []byte("abc")},
		{"empty middle", [][]byte{[]byte("x"), {}, []byte("y")}, []byte("xy")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := &Inode{}
			mntDir, _ := testMount(t, root, &Options{
				OnAdd: func(ctx context.Context) {
					n := root.EmbeddedInode()
					ch := n.NewPersistentInode(ctx, &vectorFile{vecs: tc.vecs}, StableAttr{})
					n.AddChild("file", ch, false)
				},
			})
			got, err := os.ReadFile(mntDir + "/file")
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
