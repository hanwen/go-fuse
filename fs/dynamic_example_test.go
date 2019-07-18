// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs_test

import (
	"context"
	"log"
	"os"
	"strconv"
	"syscall"

	"github.com/hanwen/go-fuse/fs"
	"github.com/hanwen/go-fuse/fuse"
)

// numberNode is a filesystem node. Composite numbers are directories,
// prime numbers are regular files.
type numberNode struct {
	fs.Inode
	num int
}

func isPrime(n int) bool {
	for i := 2; i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

func numberToMode(n int) uint32 {
	if isPrime(n) {
		return fuse.S_IFREG
	}
	return fuse.S_IFDIR
}

var _ = (fs.NodeReaddirer)((*numberNode)(nil))

func (n *numberNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	r := make([]fuse.DirEntry, 0, n.num)
	for i := 2; i < n.num; i++ {
		d := fuse.DirEntry{
			Name: strconv.Itoa(i),
			Ino:  uint64(i),
			Mode: numberToMode(i),
		}
		r = append(r, d)
	}
	return fs.NewListDirStream(r), 0
}

var _ = (fs.NodeLookuper)((*numberNode)(nil))

func (n *numberNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	i, err := strconv.Atoi(name)
	if err != nil {
		return nil, syscall.ENOENT
	}

	if i >= n.num || i <= 1 {
		return nil, syscall.ENOENT
	}

	stable := fs.StableAttr{
		Mode: numberToMode(i),
		// The child inode is identified by its Inode number.
		// If multiple concurrent lookups try to find the same
		// inode, this deduplicates
		Ino: uint64(i),
	}
	operations := &numberNode{num: i}

	// The NewInode wraps the `operations` object into an Inode.
	child := n.NewInode(ctx, operations, stable)

	// In case of concurrent lookup requests, it can happen that operations !=
	// child.Operations().
	return child, 0
}

// File systems usually cannot fit in RAM, so the kernel must discover
// the file system dynamically: as you are entering and list directory
// contents, the kernel asks the FUSE server about the files and
// directories you are busy reading/writing.
//
// The two important operations for dynamic file systems are Readdir
// (listing the contents) and Lookup (discovering individual children
// of directories).
//
// The input to a Lookup is {parent directory, name string}.
//
// Lookup, if successful, must return an *Inode. Once the Inode is
// returned to the kernel, the kernel can issue further operations,
// such as Open or Getxattr on that node.
//
// A successful Lookup also returns an EntryOut. Among others, this
// contains file attributes (mode, size, mtime, etc.).
//
// EntryOuts also contain timeouts for the attributes (of the file),
// and for the directory entry itself (a timeout for the parent-child
// relation).  default timeouts can be set using fs.Options.  libfuse
// (the C library) typically specifies 1 second timeouts for both
// attribute and directory entries.  This is a performance
// optimization: without timeouts, every operation on file "a/b/c"
// must first do lookups for "a", "a/b" and "a/b/c", which is relatively
// expensive because of context switches between the kernel and the
// FUSE process.
//
// Unsuccessful entry lookups (the ones returning ENOENT, ie. the file
// does not exist) can also be cached by specifying entry timeout
//
// FUSE supports other operations that modify the namespace. For
// example, the Symlink, Create, Mknod, Link methods all create new
// children in directories. Hence, they also return *Inode and must
// populate their fuse.EntryOut arguments.
//
// Readdir essentiallly returns a list of strings, and it is allowed
// for Readdir to return different results from Lookup. For example,
// you can return nothing for Readdir ("ls my-fuse-mount" is empty),
// while still implementing Lookup ("ls my-fuse-mount/a-specific-file"
// shows a single file).
//
// When the kernel is short on memory, it will forget cached
// information on your file system. This announced with FORGET
// messages.  There are no guarantees if or when this happens. When it
// happens, these are handled transparently by go-fuse: all Inodes
// created with NewInode are released automatically.
//
// The following example demonstrates the mechanics with a whimsical
// file system: each number is a filesystem node. Prime numbers are
// regular files. Composite numbers are directories containing all
// smaller numbers, eg.
//
//    $ ls -F  /tmp/x/6
//    2  3  4/  5
//
// the file system nodes are deduplicated using inode numbers. The
// number 2 appears in many directories, but it is actually the represented
// by the same numberNode{} object.
//
//   $ ls -i1  /tmp/x/2  /tmp/x/8/6/4/2
//   2 /tmp/x/2
//   2 /tmp/x/8/6/4/2
//
func Example_DynamicDiscovery() {
	// This is where we'll mount the FS
	mntDir := "/tmp/x"
	os.Mkdir(mntDir, 0755)
	root := &numberNode{num: 10}
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			// Set to true to see how the file system works.
			Debug: true,
		},

		// This adds read permissions to the files and
		// directories, which is necessary for doing a chdir
		// into the mount.
		DefaultPermissions: true,
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)

	// Wait until unmount before exiting
	server.Wait()
}
