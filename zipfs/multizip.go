// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zipfs

/*

This provides a practical example of mounting Go-fuse path filesystems
on top of each other.

It is a file system that configures a Zip filesystem at /zipmount when
symlinking path/to/zipfile to /config/zipmount

*/

import (
	"context"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/nodefs"
)

// MultiZipFs is a filesystem that mounts zipfiles.
type MultiZipFs struct {
	nodefs.Inode
}

func (fs *MultiZipFs) OnAdd(ctx context.Context) {
	n := fs.NewPersistentInode(ctx, &configRoot{}, nodefs.NodeAttr{Mode: syscall.S_IFDIR})

	fs.AddChild("config", n, false)
}

type configRoot struct {
	nodefs.Inode
}

var _ = (nodefs.Unlinker)((*configRoot)(nil))
var _ = (nodefs.Symlinker)((*configRoot)(nil))

func (r *configRoot) Unlink(ctx context.Context, basename string) syscall.Errno {
	if r.GetChild(basename) == nil {
		return syscall.ENOENT
	}

	// XXX RmChild should return Inode?

	_, parent := r.Parent()
	ch := parent.GetChild(basename)
	if ch == nil {
		return syscall.ENOENT
	}
	success, _ := parent.RmChild(basename)
	if !success {
		return syscall.EIO
	}

	ch.RmAllChildren()
	parent.RmChild(basename)
	parent.NotifyEntry(basename)
	return 0
}

func (r *configRoot) Symlink(ctx context.Context, target string, base string, out *fuse.EntryOut) (*nodefs.Inode, syscall.Errno) {
	root, err := NewArchiveFileSystem(target)
	if err != nil {
		log.Println("NewZipArchiveFileSystem failed.", err)
		return nil, syscall.EINVAL
	}

	_, parent := r.Parent()
	ch := r.NewPersistentInode(ctx, root, nodefs.NodeAttr{Mode: syscall.S_IFDIR})
	parent.AddChild(base, ch, false)

	link := r.NewPersistentInode(ctx, &nodefs.MemSymlink{
		Data: []byte(target),
	}, nodefs.NodeAttr{Mode: syscall.S_IFLNK})
	r.AddChild(base, link, false)
	return link, 0
}
