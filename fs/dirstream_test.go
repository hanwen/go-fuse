package fs

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type SimpleFS struct {
	Inode
}

func (fs *SimpleFS) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	const numOfEntries = 16

	entries := make([]fuse.DirEntry, 0, numOfEntries)
	for i := 0; i < numOfEntries; i++ {
		entries = append(entries, fuse.DirEntry{
			Name: fmt.Sprintf("name%04d", i),
			Mode: fuse.S_IFREG,
			Ino:  uint64(i + 100),
		})
	}
	return NewListDirStream(entries), 0
}

func TestDirSeek(t *testing.T) {
	// Create a temporary directory for mounting
	mountDir, err := os.MkdirTemp("", "go-fuse-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(mountDir)

	root := &SimpleFS{}
	server, err := Mount(mountDir, root, nil)
	if err != nil {
		t.Fatalf("failed to mount filesystem: %v", err)
	}
	defer server.Unmount()

	// use test function from dir_test.go
	testDirSeek(t, mountDir)
}
