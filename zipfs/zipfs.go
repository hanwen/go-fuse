// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zipfs

import (
	"archive/zip"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type zipRoot struct {
	fs.Inode

	zr *zip.ReadCloser
}

var _ = (fs.NodeOnAdder)((*zipRoot)(nil))

func (zr *zipRoot) OnAdd(ctx context.Context) {
	for _, f := range zr.zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dir, base := filepath.Split(filepath.Clean(f.Name))

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

// NewZipTree creates a new file-system for the zip file named name.
func NewZipTree(name string) (fs.InodeEmbedder, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}

	return &zipRoot{zr: r}, nil
}

// zipFile is a file read from a zip archive.
type zipFile struct {
	fs.Inode
	file *zip.File

	mu   sync.Mutex
	data []byte
}

var _ = (fs.NodeOpener)((*zipFile)(nil))
var _ = (fs.NodeGetattrer)((*zipFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (zf *zipFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = uint32(zf.file.Mode()) & 07777
	out.Nlink = 1
	out.Mtime = uint64(zf.file.ModTime().Unix())
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	out.Size = zf.file.UncompressedSize64
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
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
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

// Read simply returns the data that was already unpacked in the Open call
func (zf *zipFile) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(zf.data) {
		end = len(zf.data)
	}
	return fuse.ReadResultData(zf.data[off:end]), 0
}

var _ = (fs.NodeOnAdder)((*zipRoot)(nil))

func NewArchiveFileSystem(name string) (root fs.InodeEmbedder, err error) {
	switch {
	case strings.HasSuffix(name, ".zip"):
		root, err = NewZipTree(name)
	case strings.HasSuffix(name, ".tar.gz"):
		root, err = NewTarCompressedTree(name, "gz")
	case strings.HasSuffix(name, ".tar.bz2"):
		root, err = NewTarCompressedTree(name, "bz2")
	case strings.HasSuffix(name, ".tar"):
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		root = &tarRoot{rc: f}
	default:
		return nil, fmt.Errorf("unknown archive format %q", name)
	}

	if err != nil {
		return nil, err
	}

	return root, nil
}
