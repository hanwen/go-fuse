package zipfs

import (
	"fmt"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type MemFile interface {
	Stat(out *fuse.Attr)
	Data() []byte
}

type memNode struct {
	nodefs.Node
	file MemFile
}

// MemTreeFs creates a tree of internal Inodes.  Since the tree is
// loaded in memory completely at startup, it does not need to inode
// discovery through Lookup() at serve time.
type MemTreeFs struct {
	nodefs.FileSystem
	root  memNode
	files map[string]MemFile
	Name  string
}

func NewMemTreeFs() *MemTreeFs {
	return &MemTreeFs{
		FileSystem: nodefs.NewDefaultFileSystem(),
		root:       memNode{Node: nodefs.NewDefaultNode()},
	}
}

func (fs *MemTreeFs) String() string {
	return fs.Name
}

func (fs *MemTreeFs) OnMount(conn *nodefs.FileSystemConnector) {
	for k, v := range fs.files {
		fs.addFile(k, v)
	}
	fs.files = nil
}

func (fs *MemTreeFs) Root() nodefs.Node {
	return &fs.root
}

func (n *memNode) Print(indent int) {
	s := ""
	for i := 0; i < indent; i++ {
		s = s + " "
	}

	children := n.Inode().Children()
	for k, v := range children {
		if v.IsDir() {
			fmt.Println(s + k + ":")
			mn, ok := v.Node().(*memNode)
			if ok {
				mn.Print(indent + 2)
			}
		} else {
			fmt.Println(s + k)
		}
	}
}

func (n *memNode) OpenDir(context *fuse.Context) (stream []fuse.DirEntry, code fuse.Status) {
	children := n.Inode().Children()
	stream = make([]fuse.DirEntry, 0, len(children))
	for k, v := range children {
		mode := fuse.S_IFREG | 0666
		if v.IsDir() {
			mode = fuse.S_IFDIR | 0777
		}
		stream = append(stream, fuse.DirEntry{
			Name: k,
			Mode: uint32(mode),
		})
	}
	return stream, fuse.OK
}

func (n *memNode) Open(flags uint32, context *fuse.Context) (fuseFile nodefs.File, code fuse.Status) {
	if flags&fuse.O_ANYWRITE != 0 {
		return nil, fuse.EPERM
	}

	return nodefs.NewDataFile(n.file.Data()), fuse.OK
}

func (n *memNode) Deletable() bool {
	return false
}

func (n *memNode) GetAttr(out *fuse.Attr, file nodefs.File, context *fuse.Context) fuse.Status {
	if n.Inode().IsDir() {
		out.Mode = fuse.S_IFDIR | 0777
		return fuse.OK
	}
	n.file.Stat(out)
	return fuse.OK
}

func (n *MemTreeFs) addFile(name string, f MemFile) {
	comps := strings.Split(name, "/")

	node := n.root.Inode()
	for i, c := range comps {
		child := node.GetChild(c)
		if child == nil {
			fsnode := &memNode{Node: nodefs.NewDefaultNode()}
			if i == len(comps)-1 {
				fsnode.file = f
			}

			child = node.NewChild(c, fsnode.file == nil, fsnode)
		}
		node = child
	}
}
