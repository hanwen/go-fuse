// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// zipFile is a file read from a zip archive.
type zipFile struct {
	fs.Inode
	file *zip.File

	mu   sync.Mutex
	data []byte
}

// We decompress the file on demand in Open
var _ = (fs.NodeOpener)((*zipFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*zipFile)(nil))

func (zf *zipFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = zf.file.UncompressedSize64
	return 0
}

// Open lazily unpacks zip data
func (zf *zipFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	zf.mu.Lock()
	defer zf.mu.Unlock()
	if zf.data == nil {
		rc, err := zf.file.Open()
		if err != nil {
			return nil, 0, syscall.EIO
		}
		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, 0, syscall.EIO
		}

		zf.data = content
	}

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

// Read simply returns the data that was already unpacked in the Open call
func (zf *zipFile) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(zf.data) {
		end = len(zf.data)
	}
	return fuse.ReadResultData(zf.data[off:end]), fs.OK
}

// zipRoot is the root of the Zip filesystem. Its only functionality
// is populating the filesystem.
type zipRoot struct {
	fs.Inode

	zr *zip.Reader
}

// The root populates the tree in its OnAdd method
var _ = (fs.NodeOnAdder)((*zipRoot)(nil))

func (zr *zipRoot) OnAdd(ctx context.Context) {
	// OnAdd is called once we are attached to an Inode. We can
	// then construct a tree.  We construct the entire tree, and
	// we don't want parts of the tree to disappear when the
	// kernel is short on memory, so we use persistent inodes.
	for _, f := range zr.zr.File {
		dir, base := filepath.Split(f.Name)

		p := &zr.Inode
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = p.NewPersistentInode(ctx, &fs.Inode{},
					fs.StableAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}
		ch := p.NewPersistentInode(ctx, &zipFile{file: f}, fs.StableAttr{})
		p.AddChild(base, ch, true)
	}
}

// ExampleZipFS shows an in-memory, static file system
func Example_zipFS() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatal("usage: zipmount ZIP-FILE")
	}
	zfile, err := zip.OpenReader(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	root := &zipRoot{zr: &zfile.Reader}
	mnt := "/tmp/x"
	os.Mkdir(mnt, 0755)
	server, err := fs.Mount(mnt, root, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("zip file mounted")
	fmt.Printf("to unmount: fusermount -u %s\n", mnt)
	server.Wait()
}
