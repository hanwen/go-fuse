// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memfs is a writable in-memory file system for tests. It
// builds on all platforms.
package memfs

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v3/fs"
	"github.com/hanwen/go-fuse/v3/fuse"
)

// dir adds Statfs to fs.MemDir, also for its subdirectories.
type dir struct {
	*fs.MemDir
}

var (
	_ = (fs.NodeWrapChilder)((*dir)(nil))
	_ = (fs.NodeStatfser)((*dir)(nil))
)

func (d *dir) WrapChild(ctx context.Context, ops fs.InodeEmbedder) fs.InodeEmbedder {
	if n, ok := ops.(*fs.MemDir); ok {
		return &dir{n}
	}
	return ops
}

func (d *dir) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	out.Bsize = 4096
	out.Frsize = 4096
	out.Blocks = 1 << 20
	out.Bfree = 1 << 19
	out.Bavail = 1 << 19
	out.NameLen = 255
	return 0
}

// NewRoot returns the root of an empty file system.
func NewRoot() fs.InodeEmbedder {
	root := &fs.MemDir{Attr: fuse.Attr{Mode: 0755}}
	now := time.Now()
	root.Attr.SetTimes(&now, &now, &now)
	return &dir{root}
}
