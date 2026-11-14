// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package fuse

import "syscall"

// InodeNotify invalidates the information associated with the inode
// (ie. data cache, attributes, etc.)
func (s *Server) InodeNotify(node uint64, off int64, length int64) syscall.Errno {
	return s.conn.notify.InvalInode(node, off, length)
}

// EntryNotify should be used if the existence status of an entry
// within a directory changes. You should not hold any FUSE filesystem
// locks, as that can lead to deadlock.
func (s *Server) EntryNotify(parent uint64, name string) syscall.Errno {
	return s.conn.notify.InvalEntry(parent, name)
}

// DeleteNotify notifies the kernel that an entry is removed from a
// directory.  In many cases, this is equivalent to EntryNotify,
// except when the directory is in use, eg. as working directory of
// some process. You should not hold any FUSE filesystem locks, as that
// can lead to deadlock.
func (s *Server) DeleteNotify(parent uint64, child uint64, name string) syscall.Errno {
	return s.conn.notify.Delete(parent, child, name)
}

// PruneNotify asks the kernel to forget the given nodes.
func (s *Server) PruneNotify(nodes []uint64) syscall.Errno {
	return s.conn.notify.Prune(nodes)
}

// InodeNotifyStoreCache tells kernel to store data into inode's cache.
//
// This call is similar to InodeNotify, but instead of only invalidating a data
// region, it gives updated data directly to the kernel.
func (s *Server) InodeNotifyStoreCache(node uint64, offset int64, data []byte) syscall.Errno {
	return s.conn.notify.Store(node, offset, data)
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
func (s *Server) InodeRetrieveCache(node uint64, offset int64, dest []byte) (n int, st syscall.Errno) {
	return s.conn.notify.Retrieve(node, offset, dest)
}
