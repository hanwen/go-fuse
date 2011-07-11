package zipfs

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"path/filepath"
	"syscall"
	"log"
)

var _ = log.Printf

type ZipFile struct {
	*zip.File
}

func (me *ZipFile) Stat() *os.FileInfo {
	// TODO - do something intelligent with timestamps.
	return &os.FileInfo{
		Mode: syscall.S_IFREG | 0444,
		Size: int64(me.File.UncompressedSize),
	}
}

func (me *ZipFile) Data() []byte {
	data := make([]byte, me.UncompressedSize)
	zf := (*me)
	rc, err := zf.Open()
	if err != nil {
		panic("zip open")
	}

	start := 0
	for {
		n, err := rc.Read(data[start:])
		start += n
		if err == os.EOF {
			break
		}
		if err != nil && err != os.EOF {
			panic(fmt.Sprintf("read err: %v, n %v, sz %v", err, n, len(data)))
		}
	}
	return data
}


func zipFilesToTree(files []*zip.File) *MemTree {
	t := NewMemTree()
	for _, f := range files {
		parent := t
		comps := strings.Split(filepath.Clean(f.Name), "/")
		base := ""

		// Ugh - zip files have directories separate.
		if !strings.HasSuffix(f.Name, "/") {
			base = comps[len(comps)-1]
			comps = comps[:len(comps)-1]
		}
		for _, c := range comps {
			parent = parent.FindDir(c)
		}
		if base != "" {
			parent.files[base] = &ZipFile{File: f}
		}
	}
	return t
}


// NewZipTree creates a new file-system for the zip file named name.
func NewZipTree(name string) (*MemTree, os.Error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}
	return zipFilesToTree(r.File), nil
}

func NewArchiveFileSystem(name string) (fs *MemTreeFileSystem, err os.Error) {
	var tree *MemTree
	if strings.HasSuffix(name, ".zip") {
		tree, err = NewZipTree(name)
	}
	if strings.HasSuffix(name, ".tar.gz") {
		tree, err = NewTarCompressedTree(name, "gz")
	}
	if strings.HasSuffix(name, ".tar.bz2") {
		tree, err = NewTarCompressedTree(name, "bz2")
	}
	if strings.HasSuffix(name, ".tar") {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		tree = NewTarTree(f)
	}
	if err != nil {
		return nil, err
	}

	if tree == nil {
		return nil, os.NewError(fmt.Sprintf("Unknown type for %v", name))
	}

	return NewMemTreeFileSystem(tree), nil
}
