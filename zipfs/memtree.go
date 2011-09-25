package zipfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"os"
	"strings"
)

type MemFile interface {
	Stat() *os.FileInfo
	Data() []byte
}

type memNode struct {
	fuse.DefaultFsNode
	file MemFile
}

// MemTreeFs creates a tree of internal Inodes.  Since the tree is
// loaded in memory completely at startup, it does not need to inode
// discovery through Lookup() at serve time.
type MemTreeFs struct {
	fuse.DefaultNodeFileSystem
	root  memNode
	files map[string]MemFile
}

func NewMemTreeFs() *MemTreeFs {
	d := new(MemTreeFs)
	return d
}

func (me *MemTreeFs) OnMount(conn *fuse.FileSystemConnector) {
	for k, v := range me.files {
		me.addFile(k, v)
	}
	me.files = nil
}

func (me *MemTreeFs) Root() fuse.FsNode {
	return &me.root
}

func (me *memNode) Print(indent int) {
	s := ""
	for i := 0; i < indent; i++ {
		s = s + " "
	}

	children := me.Inode().Children()
	for k, v := range children {
		if v.IsDir() {
			fmt.Println(s + k + ":")
			mn, ok := v.FsNode().(*memNode)
			if ok {
				mn.Print(indent + 2)
			}
		} else {
			fmt.Println(s + k)
		}
	}
}

// We construct the tree at mount, so we never need to look anything up.
func (me *memNode) Lookup(name string, c *fuse.Context) (fi *os.FileInfo, node fuse.FsNode, code fuse.Status) {
	return nil, nil, fuse.ENOENT
}

func (me *memNode) OpenDir(context *fuse.Context) (stream chan fuse.DirEntry, code fuse.Status) {
	children := me.Inode().Children()
	stream = make(chan fuse.DirEntry, len(children))
	for k, v := range children {
		mode := fuse.S_IFREG | 0666
		if v.IsDir() {
			mode = fuse.S_IFDIR | 0777
		}
		stream <- fuse.DirEntry{
			Name: k,
			Mode: uint32(mode),
		}
	}
	close(stream)
	return stream, fuse.OK
}

func (me *memNode) Open(flags uint32, context *fuse.Context) (fuseFile fuse.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	return fuse.NewDataFile(me.file.Data()), fuse.OK
}

func (me *memNode) Deletable() bool {
	return false
}

func (me *memNode) GetAttr(file fuse.File, context *fuse.Context) (*os.FileInfo, fuse.Status) {
	if me.Inode().IsDir() {
		return &os.FileInfo{
			Mode: fuse.S_IFDIR | 0777,
		}, fuse.OK
	}

	return me.file.Stat(), fuse.OK
}

func (me *MemTreeFs) addFile(name string, f MemFile) {
	comps := strings.Split(name, "/")

	node := me.root.Inode()
	for i, c := range comps {
		child := node.GetChild(c)
		if child == nil {
			fsnode := &memNode{}
			if i == len(comps)-1 {
				fsnode.file = f
			}

			child = node.New(fsnode.file == nil, fsnode)
			node.AddChild(c, child)
		}
		node = child
	}
}
