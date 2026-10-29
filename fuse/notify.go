// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import "syscall"

// Notify holds the notifications from the file system to the kernel,
// named after the FUSE_NOTIFY_* codes. It is passed to
// RawFileSystem.OnConnect in Conn, and OnConnect may replace the
// functions, eg. to wrap them for metrics or fault injection. The
// Server methods (InodeNotify, EntryNotify, etc.) dispatch through
// the same struct.
//
// All functions are non-nil. Operations that the kernel does not
// support return ENOSYS. Replacements must be non-nil too.
type Notify struct {
	InvalInode func(node uint64, off int64, length int64) Status
	InvalEntry func(parent uint64, name string) Status
	Delete     func(parent uint64, child uint64, name string) Status
	Prune      func(nodes []uint64) Status
	Store      func(node uint64, offset int64, data []byte) Status
	Retrieve   func(node uint64, offset int64, dest []byte) (n int, st Status)

	// set if created by newNotify; receives NOTIFY_REPLY.
	notifier *notifier
}

// Backing manages backing files for passthrough
// (FUSE_DEV_IOC_BACKING_OPEN/CLOSE). It is passed to
// RawFileSystem.OnConnect in Conn, and OnConnect may replace the
// functions. The functions are non-nil, and return ENOSYS if the
// transport or the kernel does not support passthrough.
type Backing struct {
	// Open registers the given file descriptor in the kernel, so
	// the kernel can access the backing file directly for read
	// and write calls. On success a backing ID is returned.
	Open func(m *BackingMap) (int32, syscall.Errno)

	// Close unregisters a backing ID returned by Open.
	Close func(id int32) syscall.Errno
}

func enosysNotify() *Notify {
	return &Notify{
		InvalInode: func(uint64, int64, int64) Status { return ENOSYS },
		InvalEntry: func(uint64, string) Status { return ENOSYS },
		Delete:     func(uint64, uint64, string) Status { return ENOSYS },
		Prune:      func([]uint64) Status { return ENOSYS },
		Store:      func(uint64, int64, []byte) Status { return ENOSYS },
		Retrieve:   func(uint64, int64, []byte) (int, Status) { return 0, ENOSYS },
	}
}

func enosysBacking() *Backing {
	return &Backing{
		Open:  func(*BackingMap) (int32, syscall.Errno) { return 0, syscall.ENOSYS },
		Close: func(int32) syscall.Errno { return syscall.ENOSYS },
	}
}

// gate returns a copy of n with the operations that the kernel does
// not support replaced by ENOSYS stubs.
func (n *Notify) gate(k *InitIn) *Notify {
	g := *n
	stub := enosysNotify()
	if !k.SupportsNotify(NOTIFY_INVAL_INODE) {
		g.InvalInode = stub.InvalInode
	}
	if !k.SupportsNotify(NOTIFY_INVAL_ENTRY) {
		g.InvalEntry = stub.InvalEntry
	}
	if !k.SupportsNotify(NOTIFY_DELETE) {
		g.Delete = stub.Delete
	}
	if !k.SupportsNotify(NOTIFY_PRUNE) {
		g.Prune = stub.Prune
	}
	if !k.SupportsNotify(NOTIFY_STORE_CACHE) {
		g.Store = stub.Store
	}
	if !k.SupportsNotify(NOTIFY_RETRIEVE_CACHE) {
		g.Retrieve = stub.Retrieve
	}
	return &g
}

func (b *Backing) gate(k *InitIn) *Backing {
	if k.Flags64()&CAP_PASSTHROUGH == 0 {
		return enosysBacking()
	}
	g := *b
	return &g
}

// InodeNotify invalidates the information associated with the inode
// (ie. data cache, attributes, etc.)
func (ms *Server) InodeNotify(node uint64, off int64, length int64) Status {
	return ms.conn.notify.InvalInode(node, off, length)
}

// EntryNotify should be used if the existence status of an entry
// within a directory changes. You should not hold any FUSE filesystem
// locks, as that can lead to deadlock.
func (ms *Server) EntryNotify(parent uint64, name string) Status {
	return ms.conn.notify.InvalEntry(parent, name)
}

// DeleteNotify notifies the kernel that an entry is removed from a
// directory.  In many cases, this is equivalent to EntryNotify,
// except when the directory is in use, eg. as working directory of
// some process. You should not hold any FUSE filesystem locks, as that
// can lead to deadlock.
func (ms *Server) DeleteNotify(parent uint64, child uint64, name string) Status {
	return ms.conn.notify.Delete(parent, child, name)
}

// PruneNotify asks the kernel to forget the given nodes.
func (ms *Server) PruneNotify(nodes []uint64) Status {
	return ms.conn.notify.Prune(nodes)
}

// InodeNotifyStoreCache tells kernel to store data into inode's cache.
//
// This call is similar to InodeNotify, but instead of only invalidating a data
// region, it gives updated data directly to the kernel.
func (ms *Server) InodeNotifyStoreCache(node uint64, offset int64, data []byte) Status {
	return ms.conn.notify.Store(node, offset, data)
}

// InodeRetrieveCache retrieves data from kernel's inode cache.
//
// InodeRetrieveCache asks kernel to return data from its cache for inode at
// [offset:offset+len(dest)) and waits for corresponding reply. If kernel cache
// has fewer consecutive data starting at offset, that fewer amount is returned.
// In particular if inode data at offset is not cached (0, OK) is returned.
//
// The kernel returns ENOENT if it does not currently have entry for this inode
// in its dentry cache.
func (ms *Server) InodeRetrieveCache(node uint64, offset int64, dest []byte) (n int, st Status) {
	return ms.conn.notify.Retrieve(node, offset, dest)
}
