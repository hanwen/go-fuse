// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

type loopbackRoot struct {
	loopbackNode

	root string
}

func (n *loopbackRoot) GetAttr(ctx context.Context, f File, out *fuse.Attr) fuse.Status {
	var err error = nil
	st := syscall.Stat_t{}
	err = syscall.Stat(n.root, &st)
	if err != nil {
		return fuse.ToStatus(err)
	}
	out.FromStat(&st)
	return fuse.OK
}

type loopbackNode struct {
	DefaultNode

	rootNode *loopbackRoot
}

func (n *loopbackNode) path() string {
	path := InodeOf(n).Path(nil)
	return filepath.Join(n.rootNode.root, path)
}

func (n *loopbackNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	p := filepath.Join(n.path(), name)

	st := syscall.Stat_t{}
	err := syscall.Lstat(p, &st)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	out.Attr.FromStat(&st)

	ch := InodeOf(n).FindChildByOpaqueID(name, out.Attr.Ino)
	if ch != nil {
		return ch, fuse.OK
	}

	node := &loopbackNode{rootNode: n.rootNode}
	ch = n.inode().NewInode(node, out.Attr.Mode, out.Attr.Ino)
	return ch, fuse.OK
}

func (n *loopbackNode) Mknod(ctx context.Context, name string, mode, rdev uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	p := filepath.Join(n.path(), name)
	err := syscall.Mknod(p, mode, int(rdev))
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Rmdir(p)
		return nil, fuse.ToStatus(err)
	}

	out.Attr.FromStat(&st)

	node := &loopbackNode{rootNode: n.rootNode}
	ch := n.inode().NewInode(node, out.Attr.Mode, out.Attr.Ino)

	return ch, fuse.OK
}

func (n *loopbackNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
	// NOSUBMIT what about umask
	p := filepath.Join(n.path(), name)
	err := os.Mkdir(p, os.FileMode(mode))
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Rmdir(p)
		return nil, fuse.ToStatus(err)
	}

	out.Attr.FromStat(&st)

	node := &loopbackNode{rootNode: n.rootNode}
	ch := n.inode().NewInode(node, out.Attr.Mode, out.Attr.Ino)

	return ch, fuse.OK
}

func (n *loopbackNode) Rmdir(ctx context.Context, name string) fuse.Status {
	p := filepath.Join(n.path(), name)
	err := syscall.Rmdir(p)
	return fuse.ToStatus(err)
}

func (n *loopbackNode) Unlink(ctx context.Context, name string) fuse.Status {
	p := filepath.Join(n.path(), name)
	err := syscall.Unlink(p)
	return fuse.ToStatus(err)
}

func (n *loopbackNode) Rename(ctx context.Context, name string, newParent Node, newName string) fuse.Status {
	p1 := filepath.Join(n.path(), name)
	var newParentLoopback *loopbackNode
	if r, ok := newParent.(*loopbackRoot); ok {
		newParentLoopback = &r.loopbackNode
	} else {
		newParentLoopback = newParent.(*loopbackNode)
	}

	p2 := filepath.Join(newParentLoopback.path(), newName)
	err := os.Rename(p1, p2)
	return fuse.ToStatus(err)
}

func (n *loopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32) (inode *Inode, fh File, fuseFlags uint32, code fuse.Status) {
	p := filepath.Join(n.path(), name)

	f, err := os.OpenFile(p, int(flags)|os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return nil, nil, 0, fuse.ToStatus(err)
	}

	st := syscall.Stat_t{}
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		f.Close()
		return nil, nil, 0, fuse.ToStatus(err)
	}

	node := &loopbackNode{rootNode: n.rootNode}
	ch := n.inode().NewInode(node, st.Mode, st.Ino)
	return ch, NewLoopbackFile(f), 0, fuse.OK
}

func (n *loopbackNode) Open(ctx context.Context, flags uint32) (fh File, fuseFlags uint32, code fuse.Status) {
	p := n.path()
	f, err := os.OpenFile(p, int(flags), 0)
	if err != nil {
		return nil, 0, fuse.ToStatus(err)
	}
	return NewLoopbackFile(f), 0, fuse.OK
}

func (n *loopbackNode) GetAttr(ctx context.Context, f File, out *fuse.Attr) fuse.Status {
	if f != nil {
		return f.GetAttr(ctx, out)
	}

	p := n.path()

	var err error = nil
	st := syscall.Stat_t{}
	err = syscall.Lstat(p, &st)
	if err != nil {
		return fuse.ToStatus(err)
	}
	out.FromStat(&st)
	return fuse.OK
}

func NewLoopback(root string) Node {
	n := &loopbackRoot{
		root: root,
	}
	n.rootNode = n

	return n
}
