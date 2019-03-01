// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

var _ = log.Printf

type loopbackRoot struct {
	loopbackNode

	root string
}

func (n *loopbackRoot) newLoopbackNode() *loopbackNode {
	return &loopbackNode{
		rootNode:  n,
		openFiles: map[*loopbackFile]uint32{},
	}
}

func (n *loopbackRoot) GetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
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
	DefaultOperations

	rootNode *loopbackRoot

	mu sync.Mutex

	// file => openflags
	openFiles map[*loopbackFile]uint32
}

func (n *loopbackNode) Release(ctx context.Context, f FileHandle) {
	if f != nil {
		n.mu.Lock()
		defer n.mu.Unlock()
		lf := f.(*loopbackFile)
		delete(n.openFiles, lf)
		f.Release(ctx)
	}
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
	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, out.Attr.Mode, idFromStat(&st))
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

	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, out.Attr.Mode, idFromStat(&st))

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

	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, out.Attr.Mode, idFromStat(&st))

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

func (n *loopbackNode) Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) fuse.Status {

	if flags != 0 {
		return fuse.ENOSYS
	}

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

func idFromStat(st *syscall.Stat_t) FileID {
	return FileID{
		Gen: 1,
		// This should work well for traditional backing FSes,
		// not so much for other go-fuse FS-es
		Ino: uint64(st.Dev)<<32 ^ st.Ino,
	}
}

func (n *loopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32) (inode *Inode, fh FileHandle, fuseFlags uint32, code fuse.Status) {
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

	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, st.Mode, idFromStat(&st))
	lf := newLoopbackFile(f)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.openFiles[lf] = flags | syscall.O_CREAT
	return ch, lf, 0, fuse.OK
}

func (n *loopbackNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {
	p := filepath.Join(n.path(), name)
	err := syscall.Symlink(target, p)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	st := syscall.Stat_t{}
	if syscall.Lstat(p, &st); err != nil {
		syscall.Unlink(p)
		return nil, fuse.ToStatus(err)
	}
	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, st.Mode, idFromStat(&st))

	out.Attr.FromStat(&st)
	return ch, fuse.OK
}

func (n *loopbackNode) Readlink(ctx context.Context) (string, fuse.Status) {
	p := n.path()

	for l := 256; ; l *= 2 {
		buf := make([]byte, l)
		sz, err := syscall.Readlink(p, buf)
		if err != nil {
			return "", fuse.ToStatus(err)
		}

		if sz < len(buf) {
			return string(buf[:sz]), fuse.OK
		}
	}
}

func (n *loopbackNode) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, code fuse.Status) {
	p := n.path()
	f, err := os.OpenFile(p, int(flags), 0)
	if err != nil {
		return nil, 0, fuse.ToStatus(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	lf := newLoopbackFile(f)
	n.openFiles[lf] = flags
	return lf, 0, fuse.OK
}

func (n *loopbackNode) fGetAttr(ctx context.Context, out *fuse.AttrOut) (fuse.Status, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for f := range n.openFiles {
		if f != nil {
			return f.GetAttr(ctx, out), true
		}
	}
	return fuse.EBADF, false
}

func (n *loopbackNode) GetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
	if f != nil {
		// this never happens because the kernel never sends FH on getattr.
		return f.GetAttr(ctx, out)

	}
	if code, ok := n.fGetAttr(ctx, out); ok {
		return code
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

func NewLoopback(root string) Operations {
	n := &loopbackRoot{
		root: root,
	}
	n.rootNode = n
	n.openFiles = map[*loopbackFile]uint32{}
	return n
}
