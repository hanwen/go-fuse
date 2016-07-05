package test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type xattrNode struct {
	nodefs.Node
}

func (n *xattrNode) OnMount(fsConn *nodefs.FileSystemConnector) {
	n.Inode().NewChild("child", false, &xattrChildNode{nodefs.NewDefaultNode()})
}

type xattrChildNode struct {
	nodefs.Node
}

func (n *xattrChildNode) GetXAttr(attr string, context *fuse.Context) ([]byte, fuse.Status) {
	return []byte("value"), fuse.OK
}

func TestDefaultXAttr(t *testing.T) {
	dir, err := ioutil.TempDir("", "go-fuse")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(dir)

	root := &xattrNode{
		Node: nodefs.NewDefaultNode(),
	}

	opts := nodefs.NewOptions()
	opts.Debug = VerboseTest()
	s, _, err := nodefs.MountRoot(dir, root, opts)
	if err != nil {
		t.Fatalf("MountRoot: %v", err)
	}
	go s.Serve()
	defer s.Unmount()

	var data [1024]byte
	sz, err := syscall.Getxattr(filepath.Join(dir, "child"), "attr", data[:])
	if err != nil {
		t.Fatalf("Getxattr: %v", err)
	} else if val := string(data[:sz]); val != "value" {
		t.Fatalf("got %v, want 'value'", val)
	}
}
