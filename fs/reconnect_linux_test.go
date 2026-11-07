// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal/testutil"
	"golang.org/x/sys/unix"
)

type reconnectEntry struct {
	mode     uint32
	data     []byte
	children map[string]uint64
}

type reconnectTree struct {
	mu      sync.Mutex
	entries map[uint64]*reconnectEntry
}

func newReconnectTree(fileData []byte) *reconnectTree {
	return &reconnectTree{
		entries: map[uint64]*reconnectEntry{
			1: {mode: syscall.S_IFDIR, children: map[string]uint64{"dir": 2}},
			2: {mode: syscall.S_IFDIR, children: map[string]uint64{"file.txt": 3}},
			3: {mode: syscall.S_IFREG, data: fileData},
		},
	}
}

type reconnectNode struct {
	fs.Inode
	tree *reconnectTree
	ino  uint64
}

var _ = (fs.NodeLookuper)((*reconnectNode)(nil))

func (n *reconnectNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.tree.mu.Lock()
	e := n.tree.entries[n.ino]
	childIno, ok := e.children[name]
	n.tree.mu.Unlock()
	if !ok {
		return nil, syscall.ENOENT
	}
	return n.newChild(ctx, childIno, out), 0
}

var _ = (fs.NodeLookupNoder)((*reconnectNode)(nil))

func (n *reconnectNode) LookupNode(ctx context.Context, id uint64, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.tree.mu.Lock()
	_, ok := n.tree.entries[id]
	n.tree.mu.Unlock()
	if !ok {
		return nil, syscall.ENOENT
	}
	return n.newChild(ctx, id, out), 0
}

func (n *reconnectNode) newChild(ctx context.Context, ino uint64, out *fuse.EntryOut) *fs.Inode {
	n.tree.mu.Lock()
	e := n.tree.entries[ino]
	fillReconnectAttr(&out.Attr, e)
	n.tree.mu.Unlock()

	ops := &reconnectNode{tree: n.tree, ino: ino}
	return n.NewInode(ctx, ops, fs.StableAttr{Mode: e.mode, Ino: ino})
}

var _ = (fs.NodeGetattrer)((*reconnectNode)(nil))

func (n *reconnectNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.tree.mu.Lock()
	defer n.tree.mu.Unlock()
	fillReconnectAttr(&out.Attr, n.tree.entries[n.ino])
	return 0
}

func fillReconnectAttr(out *fuse.Attr, e *reconnectEntry) {
	out.Mode = e.mode
	if e.mode&syscall.S_IFDIR == 0 {
		out.Size = uint64(len(e.data))
	}
}

var _ = (fs.NodeOpener)((*reconnectNode)(nil))

func (n *reconnectNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return nil, 0, 0
}

var _ = (fs.NodeReader)((*reconnectNode)(nil))

func (n *reconnectNode) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n.tree.mu.Lock()
	data := n.tree.entries[n.ino].data
	n.tree.mu.Unlock()

	off = min(off, int64(len(data)))
	end := min(int(off)+len(dest), len(data))
	return fuse.ReadResultData(data[off:end]), 0
}

func mountReconnectTree(t *testing.T, dir string, tree *reconnectTree) *fuse.Server {
	t.Helper()
	root := &reconnectNode{tree: tree, ino: 1}
	srv, err := fs.Mount(dir, root, &fs.Options{
		ExternalNodeID: true,
		RootStableAttr: &fs.StableAttr{Ino: 1},
		MountOptions: fuse.MountOptions{
			ExtraCapabilities: fuse.CAP_EXPORT_SUPPORT,
			Debug:             testutil.VerboseTest(),
		},
	})
	if err != nil {
		t.Fatalf("mount: %v", err)
	}
	return srv
}

// TestReconnectByFileHandle exercises fs.NodeLookupNoder directly
// using name_to_handle_at(2)/open_by_handle_at(2).
func TestReconnectByFileHandle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test needs linux")
	}
	if os.Geteuid() != 0 {
		t.Skip("test needs root (CAP_DAC_READ_SEARCH, for name_to_handle_at/open_by_handle_at)")
	}
	if _, err := os.Stat("/sys/fs/fuse/connections"); err != nil {
		t.Skipf("no /sys/fs/fuse/connections: %v", err)
	}

	const fileContent = "hello from before the reconnect"
	tree := newReconnectTree([]byte(fileContent))
	dir := t.TempDir()
	filePath := filepath.Join(dir, "dir", "file.txt")

	serverA := mountReconnectTree(t, dir, tree)

	handle, _, err := unix.NameToHandleAt(unix.AT_FDCWD, filePath, 0)
	if err != nil {
		t.Skipf("NameToHandleAt(%s): %v (needs CAP_DAC_READ_SEARCH)", filePath, err)
	}

	openByHandle := func() int {
		t.Helper()
		mountFD, err := unix.Open(dir, unix.O_RDONLY, 0)
		if err != nil {
			t.Fatalf("Open(%s): %v", dir, err)
		}
		defer unix.Close(mountFD)
		fd, err := unix.OpenByHandleAt(mountFD, handle, unix.O_RDONLY)
		if err != nil {
			t.Fatalf("OpenByHandleAt: %v", err)
		}
		return fd
	}

	readFD := func(fd int) string {
		t.Helper()
		buf := make([]byte, 64)
		n, err := unix.Read(fd, buf)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		return string(buf[:n])
	}

	fdA := openByHandle()
	if got := readFD(fdA); got != fileContent {
		t.Fatalf("initial read = %q, want %q", got, fileContent)
	}
	unix.Close(fdA)

	var st syscall.Stat_t
	if err := syscall.Stat(dir, &st); err != nil {
		t.Fatalf("Stat(%s): %v", dir, err)
	}
	connID := unix.Minor(uint64(st.Dev))
	if err := os.WriteFile(fmt.Sprintf("/sys/fs/fuse/connections/%d/abort", connID), nil, 0); err != nil {
		t.Fatalf("abort connection %d: %v", connID, err)
	}
	if err := serverA.Unmount(); err != nil {
		t.Fatalf("Unmount server A: %v", err)
	}

	serverB := mountReconnectTree(t, dir, tree)
	defer serverB.Unmount()

	// Triggers a name=="." LOOKUP against server B's (empty) identity
	// table - fs.NodeLookupNoder.
	fdB := openByHandle()
	defer unix.Close(fdB)
	if got := readFD(fdB); got != fileContent {
		t.Fatalf("read after reconnect = %q, want %q", got, fileContent)
	}
}
