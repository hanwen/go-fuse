// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// bytesFileHandle is a file handle that carries separate content for
// each Open call
type bytesFileHandle struct {
	content []byte
}

// bytesFileHandle allows reads
var _ = (fs.FileReader)((*bytesFileHandle)(nil))

func (fh *bytesFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := off + int64(len(dest))
	if end > int64(len(fh.content)) {
		end = int64(len(fh.content))
	}

	// We could copy to the `dest` buffer, but since we have a
	// []byte already, return that.
	return fuse.ReadResultData(fh.content[off:end]), 0
}

// timeFile is a file that contains the wall clock time as ASCII.
type timeFile struct {
	fs.Inode
}

// timeFile implements Open
var _ = (fs.NodeOpener)((*timeFile)(nil))

func (f *timeFile) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// disallow writes
	if fuseFlags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	// capture open time
	now := time.Now().Format(time.StampNano) + "\n"
	fh = &bytesFileHandle{
		content: []byte(now),
	}

	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, 0
}

// ExampleDirectIO shows how to create a file whose contents change on
// every read.
func Example_directIO() {
	mntDir := "/tmp/x"
	root := &fs.Inode{}

	// Mount the file system
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{Debug: false},

		// Setup the clock file.
		OnAdd: func(ctx context.Context) {
			ch := root.NewPersistentInode(
				ctx,
				&timeFile{},
				fs.StableAttr{Mode: syscall.S_IFREG})
			root.AddChild("clock", ch, true)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("cat %s/clock to see the time\n", mntDir)
	fmt.Printf("Unmount by calling 'fusermount -u %s'\n", mntDir)

	// Serve the file system, until unmounted by calling fusermount -u
	server.Wait()
}
