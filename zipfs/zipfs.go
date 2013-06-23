package zipfs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var _ = log.Printf

type ZipFile struct {
	*zip.File
}

func (f *ZipFile) Stat(out *fuse.Attr) {
	// TODO - do something intelligent with timestamps.
	out.Mode = fuse.S_IFREG | 0444
	out.Size = uint64(f.File.UncompressedSize)
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

// NewZipTree creates a new file-system for the zip file named name.
func NewZipTree(name string) (map[string]MemFile, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}

	out := map[string]MemFile{}
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		n := filepath.Clean(f.Name)

		zf := &ZipFile{f}
		out[n] = zf
	}
	return out, nil
}

func NewArchiveFileSystem(name string) (mfs *MemTreeFs, err error) {
	mfs = NewMemTreeFs()
	mfs.Name = fmt.Sprintf("fs(%s)", name)

	if strings.HasSuffix(name, ".zip") {
		mfs.files, err = NewZipTree(name)
	}
	if strings.HasSuffix(name, ".tar.gz") {
		mfs.files, err = NewTarCompressedTree(name, "gz")
	}
	if strings.HasSuffix(name, ".tar.bz2") {
		mfs.files, err = NewTarCompressedTree(name, "bz2")
	}
	if strings.HasSuffix(name, ".tar") {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		mfs.files = NewTarTree(f)
	}
	if err != nil {
		return nil, err
	}

	if mfs.files == nil {
		return nil, errors.New(fmt.Sprintf("Unknown type for %v", name))
	}

	return mfs, nil
}
