// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v3/fuse"
)

// renameNoReplace is RENAME_NOREPLACE for renameat2().
const renameNoReplace = 0x1

// MemDir is a writable directory held in memory. Files, directories
// and symlinks created in it are *MemRegularFile, *MemDir and
// *MemSymlink nodes, which are persistent: they stay in the tree
// until they are removed.
//
// To customize the file system, embed *MemDir in a type that
// implements NodeWrapChilder. WrapChild receives each new child,
// fully initialized, and can return a wrapper for it. Wrapping the
// *MemDir children in the same type applies the customization to the
// whole tree:
//
//	type ReadOnlyFiles struct {
//		*fs.MemDir
//	}
//
//	func (d *ReadOnlyFiles) WrapChild(ctx context.Context, ops fs.InodeEmbedder) fs.InodeEmbedder {
//		switch n := ops.(type) {
//		case *fs.MemDir:
//			return &ReadOnlyFiles{n}
//		case *fs.MemRegularFile:
//			return &readOnlyFile{n}
//		}
//		return ops
//	}
//
//	root := &ReadOnlyFiles{&fs.MemDir{}}
type MemDir struct {
	Inode

	mu   sync.Mutex
	Attr fuse.Attr
}

var (
	_ = (NodeGetattrer)((*MemDir)(nil))
	_ = (NodeSetattrer)((*MemDir)(nil))
	_ = (NodeCreater)((*MemDir)(nil))
	_ = (NodeMkdirer)((*MemDir)(nil))
	_ = (NodeSymlinker)((*MemDir)(nil))
	_ = (NodeLinker)((*MemDir)(nil))
	_ = (NodeUnlinker)((*MemDir)(nil))
	_ = (NodeRmdirer)((*MemDir)(nil))
	_ = (NodeRenamer)((*MemDir)(nil))
	_ = (NodeFsyncer)((*MemDir)(nil))
)

func (d *MemDir) Getattr(ctx context.Context, fh FileHandle, out *fuse.AttrOut) syscall.Errno {
	d.mu.Lock()
	out.Attr = d.Attr
	d.mu.Unlock()
	out.Nlink = 2
	for _, ch := range d.Children() {
		if ch.IsDir() {
			out.Nlink++
		}
	}
	return OK
}

func (d *MemDir) Setattr(ctx context.Context, fh FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if _, ok := in.GetSize(); ok {
		return syscall.EISDIR
	}
	d.mu.Lock()
	setAttrMetadata(&d.Attr, in)
	d.mu.Unlock()
	return d.Getattr(ctx, fh, out)
}

func (d *MemDir) Fsync(ctx context.Context, fh FileHandle, flags uint32) syscall.Errno {
	return 0
}

func (d *MemDir) touch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	touchModified(&d.Attr)
}

// newAttr returns attributes for a new node with the given
// permissions, owned by the caller.
func newAttr(ctx context.Context, mode uint32) fuse.Attr {
	var a fuse.Attr
	a.Mode = mode & 07777
	if c, ok := fuse.FromContext(ctx); ok {
		a.Owner = c.Owner
	}
	now := time.Now()
	a.SetTimes(&now, &now, &now)
	return a
}

// addChild adds a new node under name, and fills out with its
// attributes.
func (d *MemDir) addChild(ctx context.Context, name string, ops InodeEmbedder, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if d.GetChild(name) != nil {
		return nil, syscall.EEXIST
	}
	ch := d.NewPersistentInode(ctx, ops, StableAttr{Mode: mode})
	if ga, ok := ch.Operations().(NodeGetattrer); ok {
		var a fuse.AttrOut
		if errno := ga.Getattr(ctx, nil, &a); errno != 0 {
			return nil, errno
		}
		out.Attr = a.Attr
	}
	d.touch()
	return ch, OK
}

func (d *MemDir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*Inode, FileHandle, uint32, syscall.Errno) {
	f := &MemRegularFile{Attr: newAttr(ctx, mode)}
	ch, errno := d.addChild(ctx, name, f, fuse.S_IFREG, out)
	return ch, nil, 0, errno
}

func (d *MemDir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	return d.addChild(ctx, name, &MemDir{Attr: newAttr(ctx, mode)}, fuse.S_IFDIR, out)
}

func (d *MemDir) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	l := &MemSymlink{
		Attr: newAttr(ctx, 0777),
		Data: []byte(target),
	}
	return d.addChild(ctx, name, l, fuse.S_IFLNK, out)
}

func (d *MemDir) Link(ctx context.Context, target InodeEmbedder, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if d.GetChild(name) != nil {
		return nil, syscall.EEXIST
	}
	ch := target.EmbeddedInode()
	if ch.IsDir() {
		return nil, syscall.EPERM
	}
	if ga, ok := target.(NodeGetattrer); ok {
		var a fuse.AttrOut
		if errno := ga.Getattr(ctx, nil, &a); errno != 0 {
			return nil, errno
		}
		out.Attr = a.Attr
	}
	d.touch()
	return ch, OK
}

func (d *MemDir) Unlink(ctx context.Context, name string) syscall.Errno {
	ch := d.GetChild(name)
	if ch == nil {
		return syscall.ENOENT
	}
	if ch.IsDir() {
		return syscall.EISDIR
	}
	d.touch()
	return OK
}

func (d *MemDir) Rmdir(ctx context.Context, name string) syscall.Errno {
	ch := d.GetChild(name)
	if ch == nil {
		return syscall.ENOENT
	}
	if !ch.IsDir() {
		return syscall.ENOTDIR
	}
	if len(ch.Children()) > 0 {
		return syscall.ENOTEMPTY
	}
	d.touch()
	return OK
}

func (d *MemDir) Rename(ctx context.Context, name string, newParent InodeEmbedder, newName string, flags uint32) syscall.Errno {
	ch := d.GetChild(name)
	if ch == nil {
		return syscall.ENOENT
	}
	newDir := newParent.EmbeddedInode()
	dst := newDir.GetChild(newName)
	switch {
	case flags&RENAME_EXCHANGE != 0:
		if dst == nil {
			return syscall.ENOENT
		}
	case dst != nil:
		if flags&renameNoReplace != 0 {
			return syscall.EEXIST
		}
		if dst.IsDir() != ch.IsDir() {
			if dst.IsDir() {
				return syscall.EISDIR
			}
			return syscall.ENOTDIR
		}
		if dst.IsDir() && len(dst.Children()) > 0 {
			return syscall.ENOTEMPTY
		}
	}

	d.touch()
	if nd, ok := newDir.Operations().(interface{ touch() }); ok && newDir != d.EmbeddedInode() {
		nd.touch()
	}
	return OK
}
