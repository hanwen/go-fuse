package zipfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"path/filepath"
	"log"
)

var _ = log.Printf

type ZipDirTree struct {
	subdirs map[string]*ZipDirTree
	files   map[string]*zip.File
}

func NewZipDirTree() *ZipDirTree {
	d := new(ZipDirTree)
	d.subdirs = make(map[string]*ZipDirTree)
	d.files = make(map[string]*zip.File)
	return d
}

func (me *ZipDirTree) Print(indent int) {
	s := ""
	for i := 0; i < indent; i++ {
		s = s + " "
	}
	for k, v := range me.subdirs {
		fmt.Println(s + k + ":")
		v.Print(indent + 2)
	}
	for k, _ := range me.files {
		fmt.Println(s + k)
	}

}

func (me *ZipDirTree) Lookup(name string) (*ZipDirTree, *zip.File) {
	if name == "" {
		return me, nil
	}
	parent := me
	comps := strings.Split(filepath.Clean(name), "/", -1)
	for _, c := range comps[:len(comps)-1] {
		parent = parent.subdirs[c]
		if parent == nil {
			return nil, nil
		}
	}
	base := comps[len(comps)-1]

	file, ok := parent.files[base]
	if ok {
		return parent, file
	}

	return parent.subdirs[base], nil
}

func (me *ZipDirTree) FindDir(name string) *ZipDirTree {
	s, ok := me.subdirs[name]
	if !ok {
		s = NewZipDirTree()
		me.subdirs[name] = s
	}
	return s
}

type ZipArchiveFileSystem struct {
	zipReader   *zip.ReadCloser
	tree        *ZipDirTree
	ZipFileName string

	fuse.DefaultFileSystem
}

func zipFilesToTree(files []*zip.File) *ZipDirTree {
	t := NewZipDirTree()
	for _, f := range files {
		parent := t
		comps := strings.Split(filepath.Clean(f.Name), "/", -1)
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
			parent.files[base] = f
		}
	}
	return t
}

// NewZipArchiveFileSystem creates a new file-system for the
// zip file named name.
func NewZipArchiveFileSystem(name string) (*ZipArchiveFileSystem, os.Error) {
	z := new(ZipArchiveFileSystem)
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}
	z.ZipFileName = name
	z.zipReader = r
	z.tree = zipFilesToTree(r.File)
	return z, nil
}

const zip_DIRMODE uint32 = fuse.S_IFDIR | 0700
const zip_FILEMODE uint32 = fuse.S_IFREG | 0600

func (me *ZipArchiveFileSystem) GetAttr(name string) (*os.FileInfo, fuse.Status) {
	dir, file := me.tree.Lookup(name)
	if dir == nil {
		return nil, fuse.ENOENT
	}

	a := &os.FileInfo{}
	if file == nil {
		a.Mode = zip_DIRMODE
	} else {
		// TODO - do something intelligent with timestamps.
		a.Mode = zip_FILEMODE
		a.Size = int64(file.UncompressedSize)
	}

	return a, fuse.OK
}

func (me *ZipArchiveFileSystem) Open(name string, flags uint32) (file fuse.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	_, zfile := me.tree.Lookup(name)
	if zfile == nil {
		return nil, fuse.ENOENT
	}
	return NewZipFile(zfile), fuse.OK
}

func (me *ZipArchiveFileSystem) OpenDir(name string) (stream chan fuse.DirEntry, code fuse.Status) {
	zdir, file := me.tree.Lookup(name)
	if file != nil {
		return nil, fuse.ENOSYS
	}
	if zdir == nil {
		panic("zdir")
	}
	stream = make(chan fuse.DirEntry)
	go func() {
		for k, _ := range zdir.files {
			stream <- fuse.DirEntry{
				Name: k,
				Mode: zip_FILEMODE,
			}
		}
		for k, _ := range zdir.subdirs {
			stream <- fuse.DirEntry{
				Name: k,
				Mode: zip_DIRMODE,
			}
		}
		close(stream)
	}()
	return stream, fuse.OK
}

////////////////////////////////////////////////////////////////
// files & dirs

type ZipFile struct {
	data []byte

	fuse.DefaultFile
}

func NewZipFile(f *zip.File) fuse.File {
	data := make([]byte, f.UncompressedSize)
	rc, err := f.Open()
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
	return fuse.NewReadOnlyFile(data)
}
