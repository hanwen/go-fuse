package zipfs

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var _ = fmt.Println

// TODO - handle symlinks.

func HeaderToFileInfo(h *tar.Header) *os.FileInfo {
	return &os.FileInfo{
		Name:     h.Name,
		Mode:     uint32(h.Mode),
		Uid:      h.Uid,
		Gid:      h.Gid,
		Size:     h.Size,
		Mtime_ns: h.Mtime,
		Atime_ns: h.Atime,
		Ctime_ns: h.Ctime,
	}
}


type TarFile struct {
	data []byte
	tar.Header
}

func (me *TarFile) Stat() *os.FileInfo {
	fi := HeaderToFileInfo(&me.Header)
	fi.Mode |= syscall.S_IFREG
	return fi
}

func (me *TarFile) Data() []byte {
	return me.data
}

func NewTarTree(r io.Reader) *MemTree {
	tree := NewMemTree()
	tr := tar.NewReader(r)

	var longName *string
	for {
		hdr, err := tr.Next()
		if err == os.EOF {
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

		comps := strings.Split(filepath.Clean(hdr.Name), "/")
		base := ""
		if !strings.HasSuffix(hdr.Name, "/") {
			base = comps[len(comps)-1]
			comps = comps[:len(comps)-1]
		}

		parent := tree
		for _, c := range comps {
			parent = parent.FindDir(c)
		}

		buf := bytes.NewBuffer(make([]byte, 0, hdr.Size))
		io.Copy(buf, tr)
		if base != "" {
			b := buf.Bytes()
			parent.files[base] = &TarFile{
				Header: *hdr,
				data:   b,
			}
		}
	}
	return tree
}

func NewTarCompressedTree(name string, format string) (*MemTree, os.Error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var stream io.Reader
	switch format {
	case "gz":
		unzip, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer unzip.Close()
		stream = unzip
	case "bz2":
		unzip := bzip2.NewReader(f)
		stream = unzip
	}

	return NewTarTree(stream), nil
}
