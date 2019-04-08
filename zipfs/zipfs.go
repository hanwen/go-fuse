// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zipfs

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
)

type ZipFile struct {
	*zip.File
}

func (f *ZipFile) Stat(out *fuse.Attr) {
}

func (f *ZipFile) Data() []byte {
	zf := (*f)
	rc, err := zf.Open()
	if err != nil {
		panic(err)
	}
	dest := bytes.NewBuffer(make([]byte, 0, f.UncompressedSize))

	_, err = io.CopyN(dest, rc, int64(f.UncompressedSize))
	if err != nil {
		panic(err)
	}
	return dest.Bytes()
}

type zipRoot struct {
	nodefs.Inode

	zr *zip.ReadCloser
}

var _ = (nodefs.OnAdder)((*zipRoot)(nil))

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
				ch = p.NewPersistentInode(ctx, &nodefs.Inode{},
					nodefs.StableAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}
		ch := p.NewPersistentInode(ctx, &zipFile{file: f}, nodefs.StableAttr{})
		p.AddChild(base, ch, true)
	}
}

// NewZipTree creates a new file-system for the zip file named name.
func NewZipTree(name string) (nodefs.InodeEmbedder, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}

	return &zipRoot{zr: r}, nil
}

// zipFile is a file read from a zip archive.
type zipFile struct {
	nodefs.Inode
	file *zip.File

	mu   sync.Mutex
	data []byte
}

var _ = (nodefs.Opener)((*zipFile)(nil))
var _ = (nodefs.Getattrer)((*zipFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (zf *zipFile) Getattr(ctx context.Context, f nodefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = uint32(zf.file.Mode()) & 07777
	out.Nlink = 1
	out.Mtime = uint64(zf.file.ModTime().Unix())
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	out.Size = zf.file.UncompressedSize64
	out.Blocks = (out.Size + 511) / 512
	return 0
}

// Open lazily unpacks zip data
func (zf *zipFile) Open(ctx context.Context, flags uint32) (nodefs.FileHandle, uint32, syscall.Errno) {
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
func (zf *zipFile) Read(ctx context.Context, f nodefs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(zf.data) {
		end = len(zf.data)
	}
	return fuse.ReadResultData(zf.data[off:end]), 0
}

var _ = (nodefs.OnAdder)((*zipRoot)(nil))

func NewArchiveFileSystem(name string) (root nodefs.InodeEmbedder, err error) {
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
