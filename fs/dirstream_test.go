package fs

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type SimpleFS struct {
	Inode
}

func (fs *SimpleFS) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	children := fs.Children()
	entries := make([]fuse.DirEntry, 0, len(children))
	for name, child := range children {
		entries = append(entries, fuse.DirEntry{
			Name: name,
			Ino:  child.StableAttr().Ino,
			Mode: child.Mode(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return NewListDirStream(entries), 0
}

func randomString() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return fmt.Sprintf("%x", buf)
}

func TestDirSeek(t *testing.T) {
	// Create a temporary directory for mounting
	mountDir, err := os.MkdirTemp("", "go-fuse-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(mountDir)

	// create random in memory entries
	numOfEntries := rand.Intn(16) + 16

	root := &SimpleFS{}
	options := &Options{
		OnAdd: func(ctx context.Context) {
			for i := 0; i < numOfEntries; i++ {
				child := root.NewPersistentInode(ctx, &Inode{}, StableAttr{})
				name := randomString()
				root.AddChild(name, child, false)
			}
		},
	}
	server, err := Mount(mountDir, root, options)
	if err != nil {
		t.Fatalf("failed to mount filesystem: %v", err)
	}
	defer server.Unmount()

	// use test function from dir_test.go
	testDirSeek(t, mountDir)
}
