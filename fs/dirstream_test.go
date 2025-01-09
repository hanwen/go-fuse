package fs

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"testing"
)

// no extra interface implemented, since childrenAsDirstream is used by default
type SimpleFS struct {
	Inode
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
	// record names, the names from a directory stream is the same as the order of creation
	names := make([]string, 0, numOfEntries)
	indices := make([]int, 0, numOfEntries)

	root := &SimpleFS{}
	options := &Options{
		OnAdd: func(ctx context.Context) {
			for i := 0; i < numOfEntries; i++ {
				child := root.NewPersistentInode(ctx, &Inode{}, StableAttr{})
				name := randomString()
				root.AddChild(name, child, false)
				names = append(names, name)
				indices = append(indices, i)
			}
		},
	}
	server, err := Mount(mountDir, root, options)
	if err != nil {
		t.Fatalf("failed to mount filesystem: %v", err)
	}
	defer server.Unmount()

	dir, err := os.Open(mountDir)
	if err != nil {
		t.Fatalf("failed to open mount directory: %v", err)
	}

	// shuffle the seek order
	rand.Shuffle(numOfEntries, func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	// read entries within the range
	for _, i := range indices {
		_, err := dir.Seek(int64(i), io.SeekStart)
		if err != nil {
			t.Fatalf("failed to seek to entry %d: %v", i, err)
		}

		// read one entry
		entry, err := dir.ReadDir(1)
		if err != nil {
			t.Fatalf("failed to read entry %d: %v", i, err)
		}

		if entry[0].Name() != names[i] {
			t.Fatalf("expected entry name to be %s, got %s", names[i], entry[0].Name())
		}
	}

	// out of range seek
	_, err = dir.Seek(-1, io.SeekStart)
	if err == nil {
		t.Fatalf("expected error on negative seek")
	}

	// it appears that the seek is only done when an entry is read if the offset is positive
	_, err = dir.Seek(int64(numOfEntries+1), io.SeekStart)
	if err == nil {
		_, err = dir.ReadDir(1)
		if err == nil || err == io.EOF {
			t.Fatalf("expected non-EOF error on out of range seek")
		}
	}

	// seek to last entry, EOF should be returned
	_, err = dir.Seek(int64(numOfEntries), io.SeekStart)
	if err != nil {
		t.Fatalf("failed to seek to last entry: %v", err)
	}
	_, err = dir.ReadDir(1)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
