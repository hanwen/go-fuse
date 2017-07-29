// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"github.com/hanwen/go-fuse/fuse"
)

type loopbackInoFileSystem struct {
	loopbackFileSystem
}

// NewLoopbackInoFileSystem is like NewLoopbackFileSystem but uses OpenDirIno
// instead of OpenDir.
func NewLoopbackInoFileSystem(root string) FileSystem {
	fs := NewLoopbackFileSystem(root).(*loopbackFileSystem)
	return &loopbackInoFileSystem{*fs}
}

func (fs *loopbackInoFileSystem) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	return nil, fuse.ENOSYS
}
