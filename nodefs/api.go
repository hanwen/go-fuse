// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package nodefs provides infrastructure to build tree-organized filesystems.
//
// A tree-organized filesystem is similar to UNIX or Plan 9 filesystem: it
// consists of nodes with each node being either a file or a directory. Files
// are located at tree leafs. A directory node can have other nodes as its
// children and refer to each child by name unique through the directory.
// There can be several paths leading from tree root to a particular node,
// known as hard-linking, for example
//
//	    root
//	    /  \
//	  dir1 dir2
//	    \  /
//	    file
//
// A /-separated string path describes location of a node in the tree. For example
//
//	dir1/file
//
// describes path root → dir1 → file.
//
// Each node is associated with integer ID uniquely identifying the node
// throughout filesystem. The tree-level structure of any filesystem is
// expressed through index-nodes (also known as "inode", see Inode) which
// describe parent/child relation in between nodes and node-ID association.
//
// A particular filesystem should provide nodes with filesystem
// operations implemented as defined by Operations interface. When
// filesystem is mounted, its root Operations is associated with root
// of the tree, and the tree is further build lazily when nodefs
// infrastructure needs to lookup children of nodes to process client
// requests. For every new Operations, the filesystem infrastructure
// automatically builds new index node and links it in the filesystem
// tree.  InodeOf can be used to get particular Inode associated with
// a Operations.
//
// XXX ^^^ inodes cleaned on cache clean (FORGET).
//
// XXX describe how to mount.
//
// XXX node example with Lookup.
//
// XXX describe how to pre-add nodes to tree.
//
package nodefs

import (
	"context"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// InodeOf returns index-node associated with filesystem node.
//
// The identity of the Inode does not change over the lifetime of
// the node object.
func InodeOf(node Operations) *Inode {
	return node.inode()
}

/*
NOSUBMIT: how to structure?

- one interface per method?
- one interface for files (getattr, read/write), one for dirs (lookup, opendir), one shared?
- one giant interface?
- use raw types as args rather than mimicking Golang signatures?
*/

// Operations is the interface that implements the filesystem.  Each
// Operations instance must embed DefaultNode.
type Operations interface {
	// setInode and inode are used by nodefs internally to link Inode to a Node.
	//
	// When a new Node instance is created, e.g. on Lookup, it has nil Inode.
	// Nodefs infrastructure will notice this and associate created Node with new Inode.
	//
	// See InodeOf for public API to retrieve an inode from Node.
	inode() *Inode
	setInode(*Inode)

	// File locking
	GetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status)
	SetLk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)
	SetLkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	GetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status

	// Lookup should find a direct child of the node by child name.
	//
	// VFS makes sure to call Lookup only once for particular (node, name)
	// pair.
	Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status)

	Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, fuse.Status)
	Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, fuse.Status)
	Rmdir(ctx context.Context, name string) fuse.Status
	Unlink(ctx context.Context, name string) fuse.Status
	Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) fuse.Status
	Create(ctx context.Context, name string, flags uint32, mode uint32) (node *Inode, fh FileHandle, fuseFlags uint32, code fuse.Status)
	Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, code fuse.Status)
	Readlink(ctx context.Context) (string, fuse.Status)
	Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, code fuse.Status)

	Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, fuse.Status)

	Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, code fuse.Status)

	Fsync(ctx context.Context, f FileHandle, flags uint32) (code fuse.Status)

	// Flush is called for close() call on a file descriptor. In
	// case of duplicated descriptor, it may be called more than
	// once for a file.
	Flush(ctx context.Context, f FileHandle) fuse.Status

	// This is called to before the file handle is forgotten. This
	// method has no return value, so nothing can synchronizes on
	// the call. Any cleanup that requires specific synchronization or
	// could fail with I/O errors should happen in Flush instead.
	Release(ctx context.Context, f FileHandle)

	/*
		NOSUBMIT - fold into a setattr method, or expand methods?

		Decoding SetAttr is a bit of a PITA, but if we use fuse
		types as args, we can't take apart SetAttr for the caller
	*/

	Truncate(ctx context.Context, f FileHandle, size uint64) fuse.Status
	Chown(ctx context.Context, f FileHandle, uid uint32, gid uint32) fuse.Status
	Chmod(ctx context.Context, f FileHandle, perms uint32) fuse.Status
	Utimens(ctx context.Context, f FileHandle, atime *time.Time, mtime *time.Time) fuse.Status
	Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) (code fuse.Status)
}

type FileHandle interface {
	Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, fuse.Status)
	Write(ctx context.Context, data []byte, off int64) (written uint32, code fuse.Status)

	// File locking
	GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status)
	SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)
	SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)

	// Flush is called for close() call on a file descriptor. In
	// case of duplicated descriptor, it may be called more than
	// once for a file.
	Flush(ctx context.Context) fuse.Status

	Fsync(ctx context.Context, flags uint32) fuse.Status

	// This is called to before the file handle is forgotten. This
	// method has no return value, so nothing can synchronizes on
	// the call. Any cleanup that requires specific synchronization or
	// could fail with I/O errors should happen in Flush instead.
	Release(ctx context.Context)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	// TODO - fold into a setattr method?
	GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status
	Truncate(ctx context.Context, size uint64) fuse.Status
	Chown(ctx context.Context, uid uint32, gid uint32) fuse.Status
	Chmod(ctx context.Context, perms uint32) fuse.Status
	Utimens(ctx context.Context, atime *time.Time, mtime *time.Time) fuse.Status
	Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (code fuse.Status)
}

type Options struct {
	Debug bool

	EntryTimeout    *time.Duration
	AttrTimeout     *time.Duration
	NegativeTimeout *time.Duration
}
