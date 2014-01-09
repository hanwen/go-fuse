package nodefs

import (
	"io/ioutil"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
)

type nodeReadNode struct {
	Node
	data []byte
}

func newNodeReadNode(d []byte) *nodeReadNode {
	return &nodeReadNode{NewDefaultNode(), d}
}

func (n *nodeReadNode) Open(flags uint32, context *fuse.Context) (file File, code fuse.Status) {
	return nil, fuse.OK
}

func (n *nodeReadNode) Read(file File, dest []byte, off int64, context *fuse.Context) (fuse.ReadResult, fuse.Status) {
	e := off + int64(len(dest))
	if int(e) > len(n.data) {
		e = int64(len(n.data))
	}
	return fuse.ReadResultData(n.data[off:int(e)]), fuse.OK
}

func (n *nodeReadNode) Lookup(out *fuse.Attr, name string, context *fuse.Context) (*Inode, fuse.Status) {
	out.Mode = fuse.S_IFREG | 0644
	out.Size = uint64(len(name))
	ch := n.Inode().NewChild(name, false, newNodeReadNode([]byte(name)))
	return ch, fuse.OK
}

func TestNodeRead(t *testing.T) {
	dir, err := ioutil.TempDir("", "nodefs")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}

	root := newNodeReadNode([]byte("root"))
	s, _, err := MountRoot(dir, root, nil)
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	s.SetDebug(true)
	go s.Serve()
	defer s.Unmount()
	content, err := ioutil.ReadFile(dir + "/file")
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	want := "file"
	if string(content) != want {
		t.Fatalf("got %q, want %q", content, want)
	}
}
