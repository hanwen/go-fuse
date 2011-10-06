package zipfs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"io"
	"os"
	"path/filepath"
	"strings"
	"log"
)

var _ = log.Printf

type ZipFile struct {
	*zip.File
}

func (me *ZipFile) Stat() *os.FileInfo {
	// TODO - do something intelligent with timestamps.
	return &os.FileInfo{
		Mode: fuse.S_IFREG | 0444,
		Size: int64(me.File.UncompressedSize),
	}
}

func (me *ZipFile) Data() []byte {
	zf := (*me)
	rc, err := zf.Open()
	if err != nil {
		panic(err)
	}
	dest := bytes.NewBuffer(make([]byte, 0, me.UncompressedSize))

	_, err = io.CopyN(dest, rc, int64(me.UncompressedSize))
	if err != nil {
		panic(err)
	}
	return dest.Bytes()
}

// NewZipTree creates a new file-system for the zip file named name.
func NewZipTree(name string) (map[string]MemFile, os.Error) {
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

func NewArchiveFileSystem(name string) (mfs *MemTreeFs, err os.Error) {
	mfs = &MemTreeFs{}
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
		return nil, os.NewError(fmt.Sprintf("Unknown type for %v", name))
	}

	return mfs, nil
}
