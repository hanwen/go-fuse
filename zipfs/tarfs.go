// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zipfs

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
)

// TODO - handle symlinks.

func HeaderToFileInfo(out *fuse.Attr, h *tar.Header) {
	out.Mode = uint32(h.Mode)
	out.Size = uint64(h.Size)
	out.Uid = uint32(h.Uid)
	out.Gid = uint32(h.Gid)
	out.SetTimes(&h.AccessTime, &h.ModTime, &h.ChangeTime)
}

type TarFile struct {
	data []byte
	tar.Header
}

func (f *TarFile) Stat(out *fuse.Attr) {
	HeaderToFileInfo(out, &f.Header)
	out.Mode |= syscall.S_IFREG
}

func (f *TarFile) Data() []byte {
	return f.data
}

type tarRoot struct {
	nodefs.Inode
	rc io.ReadCloser
}

func (r *tarRoot) OnAdd(ctx context.Context) {
	tr := tar.NewReader(r.rc)
	defer r.rc.Close()

	var longName *string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			// handle error
		}

		if hdr.Typeflag == 'L' {
			buf := bytes.NewBuffer(make([]byte, 0, hdr.Size))
			io.Copy(buf, tr)
			s := buf.String()
			longName = &s
			continue
		}

		if longName != nil {
			hdr.Name = *longName
			longName = nil
		}

		if strings.HasSuffix(hdr.Name, "/") {
			continue
		}

		buf := bytes.NewBuffer(make([]byte, 0, hdr.Size))
		io.Copy(buf, tr)
		df := &nodefs.MemRegularFile{
			Data: buf.Bytes(),
		}
		dir, base := filepath.Split(filepath.Clean(hdr.Name))

		p := r.EmbeddedInode()
		for _, comp := range strings.Split(dir, "/") {
			if len(comp) == 0 {
				continue
			}
			ch := p.GetChild(comp)
			if ch == nil {
				p.AddChild(comp, p.NewPersistentInode(ctx,
					&nodefs.Inode{},
					nodefs.NodeAttr{Mode: syscall.S_IFDIR}), false)
			}
			p = ch
		}

		HeaderToFileInfo(&df.Attr, hdr)
		p.AddChild(base, r.NewPersistentInode(ctx, df, nodefs.NodeAttr{}), false)
	}
}

type readCloser struct {
	io.Reader
	close func() error
}

func (rc *readCloser) Close() error {
	return rc.close()
}

func NewTarCompressedTree(name string, format string) (nodefs.InodeEmbedder, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var stream io.ReadCloser
	switch format {
	case "gz":
		unzip, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		stream = &readCloser{
			unzip,
			f.Close,
		}
	case "bz2":
		unzip := bzip2.NewReader(f)
		stream = &readCloser{
			unzip,
			f.Close,
		}
	}

	return &tarRoot{rc: stream}, nil
}
