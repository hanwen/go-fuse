package zipfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"strings"
	"path/filepath"
)


type MemFile interface {
	Stat() *os.FileInfo
	Data() []byte
}

type MemTree struct {
	subdirs map[string]*MemTree
	files   map[string]MemFile
}

func NewMemTree() *MemTree {
	d := new(MemTree)
	d.subdirs = make(map[string]*MemTree)
	d.files = make(map[string]MemFile)
	return d
}

func (me *MemTree) Print(indent int) {
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

func (me *MemTree) Lookup(name string) (*MemTree, MemFile) {
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

func (me *MemTree) FindDir(name string) *MemTree {
	s, ok := me.subdirs[name]
	if !ok {
		s = NewMemTree()
		me.subdirs[name] = s
	}
	return s
}


////////////////////////////////////////////////////////////////

type MemTreeFileSystem struct {
	tree *MemTree
	fuse.DefaultFileSystem
}

func NewMemTreeFileSystem(t *MemTree) *MemTreeFileSystem {
	return &MemTreeFileSystem{
		tree: t,
	}
}

const mem_DIRMODE uint32 = fuse.S_IFDIR | 0500
const mem_FILEMODE uint32 = fuse.S_IFREG | 0400

func (me *MemTreeFileSystem) GetAttr(name string) (*os.FileInfo, fuse.Status) {
	dir, file := me.tree.Lookup(name)
	if dir == nil {
		return nil, fuse.ENOENT
	}

	a := &os.FileInfo{}
	if file == nil {
		a.Mode = mem_DIRMODE
	} else {
		a = file.Stat()
	}

	return a, fuse.OK
}

func (me *MemTreeFileSystem) Open(name string, flags uint32) (fuseFile fuse.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	// TODO - should complain if it is a directory.
	_, file := me.tree.Lookup(name)
	if file == nil {
		return nil, fuse.ENOENT
	}
	return fuse.NewReadOnlyFile(file.Data()), fuse.OK
}

func (me *MemTreeFileSystem) OpenDir(name string) (stream chan fuse.DirEntry, code fuse.Status) {
	dir, file := me.tree.Lookup(name)
	if dir == nil {
		return nil, fuse.ENOENT
	}
	if file != nil {
		return nil, fuse.ENOTDIR
	}

	stream = make(chan fuse.DirEntry, len(dir.files)+len(dir.subdirs))
	for k, _ := range dir.files {
		stream <- fuse.DirEntry{
			Name: k,
			Mode: mem_FILEMODE,
		}
	}
	for k, _ := range dir.subdirs {
		stream <- fuse.DirEntry{
			Name: k,
			Mode: mem_DIRMODE,
		}
	}
	close(stream)
	return stream, fuse.OK
}
