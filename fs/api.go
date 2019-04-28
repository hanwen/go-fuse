// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fs provides infrastructure to build tree-organized filesystems.
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
// The filesystem nodes are struct that embed the Inode type, so they
// comply with the InodeEmbedder interface.  They should be
// initialized by calling NewInode or NewPersistentInode before being
// manipulated further, eg.
//
//
//  type myNode struct {
//     Inode
//  }
//
//  func (n *myNode) Lookup(ctx context.Context, name string,  ... ) (*Inode, syscall.Errno) {
//    child := myNode{}
//    return n.NewInode(ctx, &myNode{}, StableAttr{Mode: syscall.S_IFDIR}), 0
//  }
//
// On mounting, the root InodeEmbedder is associated with root of the
// tree.
//
// The kernel can evict inode data to free up memory. It does so by
// issuing FORGET calls. When a node has no children, and no kernel
// references, it is removed from the file system trees.
//
// File system trees can also be constructed in advance. This is done
// by instantiating "persistent" inodes from the OnAdder
// implementation. Persistent inodes remain in memory even if the
// kernel has forgotten them.  See zip_test.go for an example of how
// to do this.
//
// File systems whose tree structures are on backing storage typically
// discover the file system tree on-demand, and if the kernel is tight
// on memory, parts of the tree are forgotten again. These file
// systems should implement Lookuper instead.  The loopback file
// system created by `NewLoopbackRoot` provides a straightforward
// example.
//
// All error reporting must use the syscall.Errno type. The value 0
// (`OK`) should be used to indicate success. The method names are
// inspired on the system call names, so we have Listxattr rather than
// ListXAttr.
//
// Locks for networked filesystems are supported through the suite of
// Getlk, Setlk and Setlkw methods. They alllow locks on regions of
// regular files.
package fs

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// InodeEmbedder is an interface for structs that embed Inode.
//
// In general, if an InodeEmbedder does not implement specific
// filesystem methods, the filesystem will react as if it is a
// read-only filesystem with a predefined tree structure. See
// zipfs_test.go for an example.  A example is in zip_test.go
type InodeEmbedder interface {
	// populateInode and inode are used internally to link Inode
	// to a Node.
	//
	// See Inode() for the public API to retrieve an inode from Node.
	embed() *Inode

	// EmbeddedInode returns a pointer to the embedded inode.
	EmbeddedInode() *Inode
}

// Statfs implements statistics for the filesystem that holds this
// Inode. If not defined, the `out` argument will zeroed with an OK
// result.  This is because OSX filesystems must Statfs, or the mount
// will not work.
type NodeStatfser interface {
	Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno
}

// Access should return if the caller can access the file with the
// given mode.  This is used for two purposes: to determine if a user
// may enter a directory, and to answer to implement the access system
// call.  In the latter case, the context has data about the real
// UID. For example, a root-SUID binary called by user susan gets the
// UID and GID for susan here.
//
// If not defined, a default implementation will check traditional
// unix permissions of the Getattr result agains the caller. If so, it
// is necessary to either return permissions from GetAttr/Lookup or
// set Options.DefaultPermissions in order to allow chdir into the
// FUSE mount.
type NodeAccesser interface {
	Access(ctx context.Context, mask uint32) syscall.Errno
}

// GetAttr reads attributes for an Inode. The library will
// ensure that Mode and Ino are set correctly. For regular
// files, Size should be set so it can be read correctly.
type NodeGetattrer interface {
	Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno
}

// SetAttr sets attributes for an Inode.
type NodeSetattrer interface {
	Setattr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno
}

// OnAdd is called when this InodeEmbedder is initialized.
type NodeOnAdder interface {
	OnAdd(ctx context.Context)
}

// Getxattr should read data for the given attribute into
// `dest` and return the number of bytes. If `dest` is too
// small, it should return ERANGE and the size of the attribute.
// If not defined, Getxattr will return ENOATTR.
type NodeGetxattrer interface {
	Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno)
}

// Setxattr should store data for the given attribute.  See
// setxattr(2) for information about flags.
// If not defined, Setxattr will return ENOATTR.
type NodeSetxattrer interface {
	Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno
}

// Removexattr should delete the given attribute.
// If not defined, Removexattr will return ENOATTR.
type NodeRemovexattrer interface {
	Removexattr(ctx context.Context, attr string) syscall.Errno
}

// Listxattr should read all attributes (null terminated) into
// `dest`. If the `dest` buffer is too small, it should return ERANGE
// and the correct size.  If not defined, return an empty list and
// success.
type NodeListxattrer interface {
	Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno)
}

// Readlink reads the content of a symlink.
type NodeReadlinker interface {
	Readlink(ctx context.Context) ([]byte, syscall.Errno)
}

// Open opens an Inode (of regular file type) for reading. It
// is optional but recommended to return a FileHandle.
type NodeOpener interface {
	Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno)
}

// Reads data from a file. The data should be returned as
// ReadResult, which may be constructed from the incoming
// `dest` buffer. If the file was opened without FileHandle,
// the FileHandle argument here is nil. The default
// implementation forwards to the FileHandle.
type NodeReader interface {
	Read(ctx context.Context, f FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno)
}

// Writes the data into the file handle at given offset. After
// returning, the data will be reused and may not referenced.
// The default implementation forwards to the FileHandle.
type NodeWriter interface {
	Write(ctx context.Context, f FileHandle, data []byte, off int64) (written uint32, errno syscall.Errno)
}

// Fsync is a signal to ensure writes to the Inode are flushed
// to stable storage.
type NodeFsyncer interface {
	Fsync(ctx context.Context, f FileHandle, flags uint32) syscall.Errno
}

// Flush is called for close() call on a file descriptor. In
// case of duplicated descriptor, it may be called more than
// once for a file.   The default implementation forwards to the
// FileHandle.
type NodeFlusher interface {
	Flush(ctx context.Context, f FileHandle) syscall.Errno
}

// This is called to before a FileHandle is forgotten. The
// kernel ignores the return value of this method,
// so any cleanup that requires specific synchronization or
// could fail with I/O errors should happen in Flush instead.
// The default implementation forwards to the FileHandle.
type NodeReleaser interface {
	Release(ctx context.Context, f FileHandle) syscall.Errno
}

// Allocate preallocates space for future writes, so they will
// never encounter ESPACE.
type NodeAllocater interface {
	Allocate(ctx context.Context, f FileHandle, off uint64, size uint64, mode uint32) syscall.Errno
}

// CopyFileRange copies data between sections of two files,
// without the data having to pass through the calling process.
type NodeCopyFileRanger interface {
	CopyFileRange(ctx context.Context, fhIn FileHandle,
		offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
		len uint64, flags uint64) (uint32, syscall.Errno)
}

// Lseek is used to implement holes: it should return the
// first offset beyond `off` where there is data (SEEK_DATA)
// or where there is a hole (SEEK_HOLE).
type NodeLseeker interface {
	Lseek(ctx context.Context, f FileHandle, Off uint64, whence uint32) (uint64, syscall.Errno)
}

// Getlk returns locks that would conflict with the given input
// lock. If no locks conflict, the output has type L_UNLCK. See
// fcntl(2) for more information.
// If not defined, returns ENOTSUP
type NodeGetlker interface {
	Getlk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno
}

// Setlk obtains a lock on a file, or fail if the lock could not
// obtained.  See fcntl(2) for more information.  If not defined,
// returns ENOTSUP
type NodeSetlker interface {
	Setlk(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno
}

// Setlkw obtains a lock on a file, waiting if necessary. See fcntl(2)
// for more information.  If not defined, returns ENOTSUP
type NodeSetlkwer interface {
	Setlkw(ctx context.Context, f FileHandle, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno
}

// DirStream lists directory entries.
type DirStream interface {
	// HasNext indicates if there are further entries. HasNext
	// might be called on already closed streams.
	HasNext() bool

	// Next retrieves the next entry. It is only called if HasNext
	// has previously returned true.  The Errno return may be used to
	// indicate I/O errors
	Next() (fuse.DirEntry, syscall.Errno)

	// Close releases resources related to this directory
	// stream.
	Close()
}

// Lookup should find a direct child of the node by child
// name.  If the entry does not exist, it should return ENOENT
// and optionally set a NegativeTimeout in `out`. If it does
// exist, it should return attribute data in `out` and return
// the Inode for the child. A new inode can be created using
// `Inode.NewInode`. The new Inode will be added to the FS
// tree automatically if the return status is OK.
//
// If not defined, we look for an existing child with the given name,
// or returns ENOENT.
type NodeLookuper interface {
	Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno)
}

// OpenDir opens a directory Inode for reading its
// contents. The actual reading is driven from ReadDir, so
// this method is just for performing sanity/permission
// checks. The default is to return success.
type NodeOpendirer interface {
	Opendir(ctx context.Context) syscall.Errno
}

// ReadDir opens a stream of directory entries.
//
// The default ReadDir returns the list of currently known children
// from the tree
type NodeReaddirer interface {
	Readdir(ctx context.Context) (DirStream, syscall.Errno)
}

// Mkdir is similar to Lookup, but must create a directory entry and Inode.
// Default is to return EROFS.
type NodeMkdirer interface {
	Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno)
}

// Mknod is similar to Lookup, but must create a device entry and Inode.
// Default is to return EROFS.
type NodeMknoder interface {
	Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno)
}

// Link is similar to Lookup, but must create a new link to an existing Inode.
// Default is to return EROFS.
type NodeLinker interface {
	Link(ctx context.Context, target InodeEmbedder, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno)
}

// Symlink is similar to Lookup, but must create a new symbolic link.
// Default is to return EROFS.
type NodeSymlinker interface {
	Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (node *Inode, errno syscall.Errno)
}

// Create is similar to Lookup, but should create a new
// child. It typically also returns a FileHandle as a
// reference for future reads/writes.
// Default is to return EROFS.
type NodeCreater interface {
	Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno)
}

// Unlink should remove a child from this directory.  If the
// return status is OK, the Inode is removed as child in the
// FS tree automatically. Default is to return EROFS.
type NodeUnlinker interface {
	Unlink(ctx context.Context, name string) syscall.Errno
}

// Rmdir is like Unlink but for directories.
// Default is to return EROFS.
type NodeRmdirer interface {
	Rmdir(ctx context.Context, name string) syscall.Errno
}

// Rename should move a child from one directory to a different
// one. The change is effected in the FS tree if the return status is
// OK. Default is to return EROFS.
type NodeRenamer interface {
	Rename(ctx context.Context, name string, newParent InodeEmbedder, newName string, flags uint32) syscall.Errno
}

// FileHandle is a resource identifier for opened files.  FileHandles
// are useful in two cases: First, if the underlying storage systems
// needs a handle for reading/writing. See the function
// `NewLoopbackFile` for an example. Second, it is useful for
// implementing files whose contents are not tied to an inode. For
// example, a file like `/proc/interrupts` has no fixed content, but
// changes on each open call. This means that each file handle must
// have its own view of the content; this view can be tied to a
// FileHandle. Files that have such dynamic content should return the
// FOPEN_DIRECT_IO flag from their `Open` method. See directio_test.go
// for an example.
type FileHandle interface {
}

// See NodeReleaser.
type FileReleaser interface {
	Release(ctx context.Context) syscall.Errno
}

// See NodeGetattrer.
type FileGetattrer interface {
	Getattr(ctx context.Context, out *fuse.AttrOut) syscall.Errno
}

// See NodeReader.
type FileReader interface {
	Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno)
}

// See NodeWriter.
type FileWriter interface {
	Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno)
}

// See NodeGetlker.
type FileGetlker interface {
	Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) syscall.Errno
}

// See NodeSetlker.
type FileSetlker interface {
	Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno
}

// See NodeSetlkwer.
type FileSetlkwer interface {
	Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) syscall.Errno
}

// See NodeLseeker.
type FileLseeker interface {
	Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno)
}

// See NodeFlusher.
type FileFlusher interface {
	Flush(ctx context.Context) syscall.Errno
}

// See NodeFsync.
type FileFsyncer interface {
	Fsync(ctx context.Context, flags uint32) syscall.Errno
}

// See NodeFsync.
type FileSetattrer interface {
	Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno
}

// See NodeAllocater.
type FileAllocater interface {
	Allocate(ctx context.Context, off uint64, size uint64, mode uint32) syscall.Errno
}

// Options sets options for the entire filesystem
type Options struct {
	// MountOptions contain the options for mounting the fuse server
	fuse.MountOptions

	// If set to nonnil, this defines the overall entry timeout
	// for the file system. See fuse.EntryOut for more information.
	EntryTimeout *time.Duration

	// If set to nonnil, this defines the overall attribute
	// timeout for the file system. See fuse.EntryOut for more
	// information.
	AttrTimeout *time.Duration

	// If set to nonnil, this defines the overall entry timeout
	// for failed lookups (fuse.ENOENT). See fuse.EntryOut for
	// more information.
	NegativeTimeout *time.Duration

	// Automatic inode numbers are handed out sequentially
	// starting from this number. If unset, use 2^63.
	FirstAutomaticIno uint64

	// OnAdd is an alternative way to specify the OnAdd
	// functionality of the root node.
	OnAdd func(ctx context.Context)

	// DefaultPermissions sets null file permissions to 755 (dirs)
	// or 644 (other files.)
	DefaultPermissions bool

	// If nonzero, replace default (zero) UID with the given UID
	UID uint32

	// If nonzero, replace default (zero) GID with the given GID
	GID uint32
}
