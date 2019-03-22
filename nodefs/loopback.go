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
	"github.com/hanwen/go-fuse/internal"
	"golang.org/x/sys/unix"
)

type loopbackRoot struct {
	loopbackNode

	root    string
	rootDev uint64
}

func (n *loopbackRoot) newLoopbackNode() *loopbackNode {
	return &loopbackNode{
		rootNode: n,
	}
}

func (n *loopbackNode) StatFs(ctx context.Context, out *fuse.StatfsOut) fuse.Status {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(n.path(), &s)
	if err != nil {
		return fuse.ToStatus(err)
	}
	out.FromStatfsT(&s)
	return fuse.OK
}

func (n *loopbackRoot) GetAttr(ctx context.Context, out *fuse.AttrOut) fuse.Status {
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
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))
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
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))

	return ch, fuse.OK
}

func (n *loopbackNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, fuse.Status) {
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
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))

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

func toLoopbackNode(op Operations) *loopbackNode {
	if r, ok := op.(*loopbackRoot); ok {
		return &r.loopbackNode
	}
	return op.(*loopbackNode)
}

func (n *loopbackNode) Rename(ctx context.Context, name string, newParent Operations, newName string, flags uint32) fuse.Status {
	newParentLoopback := toLoopbackNode(newParent)
	if flags&unix.RENAME_EXCHANGE != 0 {
		return n.renameExchange(name, newParentLoopback, newName)
	}

	p1 := filepath.Join(n.path(), name)

	p2 := filepath.Join(newParentLoopback.path(), newName)
	err := os.Rename(p1, p2)
	return fuse.ToStatus(err)
}

func (r *loopbackRoot) idFromStat(st *syscall.Stat_t) NodeAttr {
	// We compose an inode number by the underlying inode, and
	// mixing in the device number. In traditional filesystems,
	// the inode numbers are small. The device numbers are also
	// small (typically 16 bit). Finally, we mask out the root
	// device number of the root, so a loopback FS that does not
	// encompass multiple mounts will reflect the inode numbers of
	// the underlying filesystem
	swapped := (st.Dev << 32) | (st.Dev >> 32)
	swappedRootDev := (r.rootDev << 32) | (r.rootDev >> 32)
	return NodeAttr{
		Mode: st.Mode,
		Gen:  1,
		// This should work well for traditional backing FSes,
		// not so much for other go-fuse FS-es
		Ino: (swapped ^ swappedRootDev) ^ st.Ino,
	}
}

func (n *loopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32) (inode *Inode, fh FileHandle, fuseFlags uint32, status fuse.Status) {
	p := filepath.Join(n.path(), name)

	fd, err := syscall.Open(p, int(flags)|os.O_CREATE, mode)
	if err != nil {
		return nil, nil, 0, fuse.ToStatus(err)
	}

	st := syscall.Stat_t{}
	if err := syscall.Fstat(fd, &st); err != nil {
		syscall.Close(fd)
		return nil, nil, 0, fuse.ToStatus(err)
	}

	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))
	lf := NewLoopbackFile(fd)
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
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))

	out.Attr.FromStat(&st)
	return ch, fuse.OK
}

func (n *loopbackNode) Link(ctx context.Context, target Operations, name string, out *fuse.EntryOut) (*Inode, fuse.Status) {

	p := filepath.Join(n.path(), name)
	targetNode := toLoopbackNode(target)
	err := syscall.Link(targetNode.path(), p)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	st := syscall.Stat_t{}
	if syscall.Lstat(p, &st); err != nil {
		syscall.Unlink(p)
		return nil, fuse.ToStatus(err)
	}
	node := n.rootNode.newLoopbackNode()
	ch := n.inode().NewInode(node, n.rootNode.idFromStat(&st))

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

func (n *loopbackNode) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, status fuse.Status) {
	p := n.path()
	f, err := syscall.Open(p, int(flags), 0)
	if err != nil {
		return nil, 0, fuse.ToStatus(err)
	}
	lf := NewLoopbackFile(f)
	return lf, 0, fuse.OK
}

func (n *loopbackNode) OpenDir(ctx context.Context) fuse.Status {
	fd, err := syscall.Open(n.path(), syscall.O_DIRECTORY, 0755)
	if err != nil {
		return fuse.ToStatus(err)
	}
	syscall.Close(fd)
	return fuse.OK
}

func (n *loopbackNode) ReadDir(ctx context.Context) (DirStream, fuse.Status) {
	return NewLoopbackDirStream(n.path())
}

func (n *loopbackNode) FGetAttr(ctx context.Context, f FileHandle, out *fuse.AttrOut) fuse.Status {
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

// NewLoopback returns a root node for a loopback file system whose
// root is at the given root.
func NewLoopbackRoot(root string) (DirOperations, error) {
	var st syscall.Stat_t
	err := syscall.Stat(root, &st)
	if err != nil {
		return nil, err
	}

	n := &loopbackRoot{
		root:    root,
		rootDev: st.Dev,
	}
	n.rootNode = n
	return n, nil
}

func (n *loopbackNode) Access(ctx context.Context, mask uint32) fuse.Status {
	caller, ok := fuse.FromContext(ctx)
	if !ok {
		return fuse.EACCES
	}

	var st syscall.Stat_t
	if err := syscall.Stat(n.path(), &st); err != nil {
		return fuse.ToStatus(err)
	}

	if !internal.HasAccess(caller.Uid, caller.Gid, st.Uid, st.Gid, uint32(st.Mode), mask) {
		return fuse.EACCES
	}
	return fuse.OK
}
