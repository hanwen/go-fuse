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

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// numberNode is a filesystem node representing an integer. Prime
// numbers are regular files, while composite numbers are directories
// containing all smaller numbers, eg.
//
//	$ ls -F  /tmp/x/6
//	2  3  4/  5
//
// Prime numbers are deduplicated using inode numbers. The number 2
// appears in many directories, but it is actually represented by the
// same numberNode{} object, with inode number 2, ie. it is hard-linked:
//
//	$ ls -i1  /tmp/x/2  /tmp/x/8/6/4/2
//	2 /tmp/x/2
//	2 /tmp/x/8/6/4/2
//
// Directories can have only one parent, so composite numbers are not
// deduplicated: 4 in /tmp/x/6 and 4 in /tmp/x/8 are different
// directories, with different inode numbers.
type numberNode struct {
	// Must embed an Inode for the struct to work as a node.
	fs.Inode

	// num is the integer represented in this file/directory
	num int
}

// isPrime returns whether n is prime
func isPrime(n int) bool {
	for i := 2; i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// numberToIno returns the inode number for n: n itself for primes, so
// they are hard-linked, and 0 (automatically assigned) for directories.
func numberToIno(n int) uint64 {
	if isPrime(n) {
		return uint64(n)
	}
	return 0
}

func numberToMode(n int) uint32 {
	// prime numbers are files
	if isPrime(n) {
		return fuse.S_IFREG
	}
	// composite numbers are directories
	return fuse.S_IFDIR
}

// Ensure we are implementing the NodeReaddirer interface
var _ = (fs.NodeReaddirer)((*numberNode)(nil))

// Readdir is part of the NodeReaddirer interface
func (n *numberNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	r := make([]fuse.DirEntry, 0, n.num)
	for i := 2; i < n.num; i++ {
		d := fuse.DirEntry{
			Name: strconv.Itoa(i),
			Ino:  numberToIno(i),
			Mode: numberToMode(i),
		}
		r = append(r, d)
	}
	return fs.NewListDirStream(r), 0
}

// Ensure we are implementing the NodeLookuper interface
var _ = (fs.NodeLookuper)((*numberNode)(nil))

// Lookup is part of the NodeLookuper interface
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
		// A prime is identified by its Inode number. If
		// lookups in different directories find the same
		// prime, they are deduplicated on this key. Composites
		// get Ino 0, so each lookup creates a new directory.
		Ino: numberToIno(i),
	}
	operations := &numberNode{num: i}

	// The NewInode call wraps the `operations` object into an Inode.
	child := n.NewInode(ctx, operations, stable)

	// In case of concurrent lookup requests, it can happen that operations !=
	// child.Operations().
	return child, 0
}

// ExampleDynamic is a whimsical example of a dynamically discovered
// file system.
func Example_dynamic() {
	// This is where we'll mount the FS
	mntDir := "/tmp/x"
	os.Mkdir(mntDir, 0755)
	root := &numberNode{num: 10}
	server, err := fs.Mount(mntDir, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			// Set to true to see how the file system works.
			Debug: true,
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Mounted on %s", mntDir)
	log.Printf("Unmount by calling 'fusermount -u %s'", mntDir)

	// Wait until unmount before exiting
	server.Wait()
}
