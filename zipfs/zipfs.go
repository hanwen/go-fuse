package main

import (
	"github.com/hanwen/go-fuse/fuse"
	"archive/zip"
	"fmt"
	"os"
	"flag"
	"expvar"
	"strings"
	"path"
	"log"
)

var _ = log.Printf

////////////////////////////////////////////////////////////////
// DummyPathFuse

type DirTree struct {
	subdirs map[string]*DirTree
	files map[string]*zip.File
}

func NewDirTree() *DirTree {
	d := new(DirTree)
	d.subdirs = make(map[string]*DirTree)
	d.files = make(map[string]*zip.File)
	return d
}

func (me *DirTree) Print(indent int) {
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

func (me *DirTree) Lookup(name string) (*DirTree, *zip.File) {
	if name == "" {
		return me, nil
	}
	parent := me
	comps := strings.Split(path.Clean(name), "/", -1)
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

func (me *DirTree) FindDir(name string) *DirTree {
	s, ok := me.subdirs[name]
	if !ok {
		s = NewDirTree()
		me.subdirs[name] = s
	}
	return s
}

type ZipFileFuse struct {
	zipReader *zip.Reader
	tree *DirTree
}

func FilesToTree(files []*zip.File) *DirTree {
	t := NewDirTree()
	for _, f := range files {
		parent := t
		comps := strings.Split(path.Clean(f.Name), "/", -1)
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

func NewZipFileFuse(name string) *ZipFileFuse {
	z := new(ZipFileFuse)
	r, err := zip.OpenReader(name)
	if err != nil {
		panic("zip open error")
	}
	z.zipReader = r
	z.tree = FilesToTree(r.File)
	return z
}

const DIRMODE uint32 = fuse.S_IFDIR | 0700
const FILEMODE uint32 = fuse.S_IFREG | 0600

func (self *ZipFileFuse) GetAttr(name string) (*fuse.Attr, fuse.Status) {
	dir, file := self.tree.Lookup(name)
	if dir == nil {
		return nil, fuse.ENOENT
	}

	a := new(fuse.Attr)
	if file == nil {
		a.Mode = DIRMODE
	} else {
		a.Mode = FILEMODE
		a.Size = uint64(file.UncompressedSize)
	}
		
	return a, fuse.OK
}

func (self *ZipFileFuse) Open(name string, flags uint32) (file fuse.RawFuseFile, code fuse.Status) {
	_, zfile := self.tree.Lookup(name)
	if zfile == nil {
		return nil, fuse.ENOENT
	}
	return NewZipFile(zfile), fuse.OK
}

func (self *ZipFileFuse) OpenDir(name string) (stream chan fuse.DirEntry, code fuse.Status) {
	zdir, file := self.tree.Lookup(name)
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
			Mode: FILEMODE,
			}
		}
		for k, _ := range zdir.subdirs {
			stream <- fuse.DirEntry{
			Name: k,
			Mode: DIRMODE,
			}
		}

		
		close(stream)
	}()
	return stream, fuse.OK
}

////////////////////////////////////////////////////////////////
// files & dirs

type ZipFile struct {
	file *zip.File
	data []byte
}

func NewZipFile(f *zip.File) *ZipFile {
	z := ZipFile{
		file: f,
		data: make([]byte, f.UncompressedSize),
	}
	rc, err := f.Open()
	if err != nil {
		panic("zip open")
	}

	start := 0
	for {
		n, err := rc.Read(z.data[start:])
		start += n
		if (err == os.EOF) {
			break;
		}
		if (err != nil && err != os.EOF) {
			panic(fmt.Sprintf("read err: %v, n %v, sz %v", err, n, len(z.data)))
		}
	}
	return &z
}

func (self *ZipFile) Read(input *fuse.ReadIn, bp *fuse.BufferPool) ([]byte, fuse.Status) {
	end := int(input.Offset)+int(input.Size)
	if end > len(self.data) {
		end = len(self.data)
	}
	
	// TODO - robustify bufferpool
	return self.data[input.Offset:end], fuse.OK
}

func (self *ZipFile) Write(*fuse.WriteIn, []byte) (uint32, fuse.Status) {
	return 0, fuse.ENOSYS
}

func (self *ZipFile) Flush() fuse.Status {
	return fuse.ENOSYS
}

func (self *ZipFile) Release() {

}

func (self *ZipFile) Fsync(*fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOSYS
}

type ZipDir struct {
	names []string
	modes []uint32
	nextread int
}

func (self *ZipDir) ReadDir(input *fuse.ReadIn) (*fuse.DirEntryList, fuse.Status) {
	list := fuse.NewDirEntryList(int(input.Size))
	for self.nextread < len(self.modes) {
		// TODO - fix FUSE_UNKNOWN_INO
		if list.AddString(self.names[self.nextread],
			fuse.FUSE_UNKNOWN_INO, self.modes[self.nextread]) {
			self.nextread++
		} else {
			break;
		}
	}
	return list, fuse.OK
}

func (self *ZipDir) ReleaseDir() {

}

func (self *ZipDir) FsyncDir(input *fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOSYS
}

////////////////////////////////////////////////////////////////
// unimplemented

func (self *ZipFileFuse) Readlink(name string) (string, fuse.Status) {
	return "", fuse.ENOSYS
}

func (self *ZipFileFuse) Mknod(name string, mode uint32, dev uint32) fuse.Status {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Mkdir(name string, mode uint32) fuse.Status {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Unlink(name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Rmdir(name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Symlink(value string, linkName string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Rename(oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Link(oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Chmod(name string, mode uint32) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Chown(name string, uid uint32, gid uint32) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Truncate(name string, offset uint64) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Mount(conn *fuse.PathFileSystemConnector) (fuse.Status) {
	return fuse.OK
}

func (self *ZipFileFuse) Unmount() {
}

func (self *ZipFileFuse) Access(name string, mode uint32) (code fuse.Status) {
	return fuse.ENOSYS
}

func (self *ZipFileFuse) Create(name string, flags uint32, mode uint32) (file fuse.RawFuseFile, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (self *ZipFileFuse) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code fuse.Status) {
	return fuse.ENOSYS
}

////////////////


func main() {
	// Scans the arg list and sets up flags
	flag.Parse()
	if flag.NArg() < 2 {
		// TODO - where to get program name?
		fmt.Println("usage: main ZIPFILE MOUNTPOINT")
		os.Exit(2)
	}

	orig := flag.Arg(0)
	fs := NewZipFileFuse(orig)
	conn := fuse.NewPathFileSystemConnector(fs)
	state := fuse.NewMountState(conn)

	mountPoint := flag.Arg(1)
	state.Debug = true
	state.Mount(mountPoint)

	fmt.Printf("Mounted %s on %s\n", orig, mountPoint)
	state.Loop(true)
	
	fmt.Println("Finished", state.Stats())
	for v := range(expvar.Iter()) {
		if strings.HasPrefix(v.Key, "mount") {
			fmt.Printf("%v: %v\n", v.Key, v.Value)
		}
	}
}
