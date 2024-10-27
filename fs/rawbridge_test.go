package fs

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// CustomFS is our file system structure.
type CustomFS struct {
	Inode
}

// Implement Create method for CustomFS
func (f *CustomFS) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	child := f.NewPersistentInode(ctx, &Inode{}, StableAttr{Mode: fuse.S_IFREG})
	return child, nil, 0, syscall.F_OK
}

func TestFileSystem(t *testing.T) {
	// Create a temporary directory for mounting
	mountDir, err := os.MkdirTemp("", "go-fuse-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(mountDir)

	// Create a file system instance
	root := &CustomFS{}
	opts := &Options{}
	opts.MountOptions.Options = append(opts.MountOptions.Options, "rw")
	server, err := Mount(mountDir, root, &Options{})
	if err != nil {
		t.Fatalf("failed to mount filesystem: %v", err)
	}
	defer server.Unmount()

	// Waiting for the file system to mount
	time.Sleep(100 * time.Millisecond)

	// Checking file creation
	filePath := mountDir + "/test.file"
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file in mounted filesystem: %v", err)
	}
	file.Close()

	// Check that the file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("file not created in mounted filesystem")
	}
}
