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
	InvalInode func(node uint64, off int64, length int64) syscall.Errno
	InvalEntry func(parent uint64, name string) syscall.Errno
	Delete     func(parent uint64, child uint64, name string) syscall.Errno
	Prune      func(nodes []uint64) syscall.Errno
	Store      func(node uint64, offset int64, data []byte) syscall.Errno
	Retrieve   func(node uint64, offset int64, dest []byte) (n int, st syscall.Errno)

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
		InvalInode: func(uint64, int64, int64) syscall.Errno { return ENOSYS },
		InvalEntry: func(uint64, string) syscall.Errno { return ENOSYS },
		Delete:     func(uint64, uint64, string) syscall.Errno { return ENOSYS },
		Prune:      func([]uint64) syscall.Errno { return ENOSYS },
		Store:      func(uint64, int64, []byte) syscall.Errno { return ENOSYS },
		Retrieve:   func(uint64, int64, []byte) (int, syscall.Errno) { return 0, ENOSYS },
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
	if !k.supportsNotify(notifyInvalInode) {
		g.InvalInode = stub.InvalInode
	}
	if !k.supportsNotify(notifyInvalEntry) {
		g.InvalEntry = stub.InvalEntry
	}
	if !k.supportsNotify(notifyDelete) {
		g.Delete = stub.Delete
	}
	if !k.supportsNotify(notifyPrune) {
		g.Prune = stub.Prune
	}
	if !k.supportsNotify(notifyStore) {
		g.Store = stub.Store
	}
	if !k.supportsNotify(notifyRetrieve) {
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
