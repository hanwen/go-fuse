// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// loopbackRoot holds the parameters for creating a new loopback
// filesystem. Loopback filesystem delegate their operations to an
// underlying POSIX file system.
type loopbackRoot struct {
	// The path to the root of the underlying file system.
	Path string

	// The device on which the Path resides. This must be set if
	// the underlying filesystem crosses file systems.
	Dev uint64

	// NewNode returns a new InodeEmbedder to be used to respond
	// to a LOOKUP/CREATE/MKDIR/MKNOD opcode. If not set, use a
	// LoopbackNode.
	NewNode func(rootData *loopbackRoot) InodeEmbedder
}

func (r *loopbackRoot) newNode() InodeEmbedder {
	if r.NewNode != nil {
		return r.NewNode(r)
	}
	return &loopbackNode{
		RootData: r,
	}
}

func (r *loopbackRoot) idFromStat(st *syscall.Stat_t) StableAttr {
	// We compose an inode number by the underlying inode, and
	// mixing in the device number. In traditional filesystems,
	// the inode numbers are small. The device numbers are also
	// small (typically 16 bit). Finally, we mask out the root
	// device number of the root, so a loopback FS that does not
	// encompass multiple mounts will reflect the inode numbers of
	// the underlying filesystem
	swapped := (uint64(st.Dev) << 32) | (uint64(st.Dev) >> 32)
	swappedRootDev := (r.Dev << 32) | (r.Dev >> 32)
	return StableAttr{
		Mode: uint32(st.Mode),
		Gen:  1,
		// This should work well for traditional backing FSes,
		// not so much for other go-fuse FS-es
		Ino: (swapped ^ swappedRootDev) ^ st.Ino,
	}
}

// LoopbackNode is a filesystem node in a loopback file system.
type loopbackNode struct {
	Inode

	RootData *loopbackRoot
}

var _ = (NodeStatfser)((*loopbackNode)(nil))
var _ = (NodeStatfser)((*loopbackNode)(nil))
var _ = (NodeGetattrer)((*loopbackNode)(nil))
var _ = (NodeGetxattrer)((*loopbackNode)(nil))
var _ = (NodeSetxattrer)((*loopbackNode)(nil))
var _ = (NodeRemovexattrer)((*loopbackNode)(nil))
var _ = (NodeListxattrer)((*loopbackNode)(nil))
var _ = (NodeReadlinker)((*loopbackNode)(nil))
var _ = (NodeOpener)((*loopbackNode)(nil))
var _ = (NodeCopyFileRanger)((*loopbackNode)(nil))
var _ = (NodeLookuper)((*loopbackNode)(nil))
var _ = (NodeOpendirer)((*loopbackNode)(nil))
var _ = (NodeReaddirer)((*loopbackNode)(nil))
var _ = (NodeMkdirer)((*loopbackNode)(nil))
var _ = (NodeMknoder)((*loopbackNode)(nil))
var _ = (NodeLinker)((*loopbackNode)(nil))
var _ = (NodeSymlinker)((*loopbackNode)(nil))
var _ = (NodeUnlinker)((*loopbackNode)(nil))
var _ = (NodeRmdirer)((*loopbackNode)(nil))
var _ = (NodeRenamer)((*loopbackNode)(nil))

func (n *loopbackNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(n.path(), &s)
	if err != nil {
		return ToErrno(err)
	}
	out.FromStatfsT(&s)
	return OK
}

func (n *loopbackNode) path() string {
	path := n.Path(n.Root())
	return filepath.Join(n.RootData.Path, path)
}

func (n *loopbackNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	p := filepath.Join(n.path(), name)

	st := syscall.Stat_t{}
	err := syscall.Lstat(p, &st)
	if err != nil {
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)
	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))
	return ch, 0
}

// preserveOwner sets uid and gid of `path` according to the caller information
// in `ctx`.
func (n *loopbackNode) preserveOwner(ctx context.Context, path string) error {
	if os.Getuid() != 0 {
		return nil
	}
	caller, ok := fuse.FromContext(ctx)
	if !ok {
		return nil
	}
	return syscall.Lchown(path, int(caller.Uid), int(caller.Gid))
}

func (n *loopbackNode) Mknod(ctx context.Context, name string, mode, rdev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	p := filepath.Join(n.path(), name)
	err := syscall.Mknod(p, mode, int(rdev))
	if err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, p)
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Rmdir(p)
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)

	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

	return ch, 0
}

func (n *loopbackNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	p := filepath.Join(n.path(), name)
	err := os.Mkdir(p, os.FileMode(mode))
	if err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, p)
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Rmdir(p)
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)

	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

	return ch, 0
}

func (n *loopbackNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	p := filepath.Join(n.path(), name)
	err := syscall.Rmdir(p)
	return ToErrno(err)
}

func (n *loopbackNode) Unlink(ctx context.Context, name string) syscall.Errno {
	p := filepath.Join(n.path(), name)
	err := syscall.Unlink(p)
	return ToErrno(err)
}

func (n *loopbackNode) Rename(ctx context.Context, name string, newParent InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if flags&RENAME_EXCHANGE != 0 {
		return n.renameExchange(name, newParent, newName)
	}

	p1 := filepath.Join(n.path(), name)
	p2 := filepath.Join(n.RootData.Path, newParent.EmbeddedInode().Path(nil), newName)

	err := syscall.Rename(p1, p2)
	return ToErrno(err)
}

var _ = (NodeCreater)((*loopbackNode)(nil))

func (n *loopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	p := filepath.Join(n.path(), name)
	flags = flags &^ syscall.O_APPEND
	fd, err := syscall.Open(p, int(flags)|os.O_CREATE, mode)
	if err != nil {
		return nil, nil, 0, ToErrno(err)
	}
	n.preserveOwner(ctx, p)
	st := syscall.Stat_t{}
	if err := syscall.Fstat(fd, &st); err != nil {
		syscall.Close(fd)
		return nil, nil, 0, ToErrno(err)
	}

	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))
	lf := NewLoopbackFile(fd)

	out.FromStat(&st)
	return ch, lf, 0, 0
}

func (n *loopbackNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	p := filepath.Join(n.path(), name)
	err := syscall.Symlink(target, p)
	if err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, p)
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Unlink(p)
		return nil, ToErrno(err)
	}
	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

	out.Attr.FromStat(&st)
	return ch, 0
}

func (n *loopbackNode) Link(ctx context.Context, target InodeEmbedder, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {

	p := filepath.Join(n.path(), name)
	err := syscall.Link(filepath.Join(n.RootData.Path, target.EmbeddedInode().Path(nil)), p)
	if err != nil {
		return nil, ToErrno(err)
	}
	st := syscall.Stat_t{}
	if err := syscall.Lstat(p, &st); err != nil {
		syscall.Unlink(p)
		return nil, ToErrno(err)
	}
	node := n.RootData.newNode()
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

	out.Attr.FromStat(&st)
	return ch, 0
}

func (n *loopbackNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	p := n.path()

	for l := 256; ; l *= 2 {
		buf := make([]byte, l)
		sz, err := syscall.Readlink(p, buf)
		if err != nil {
			return nil, ToErrno(err)
		}

		if sz < len(buf) {
			return buf[:sz], 0
		}
	}
}

func (n *loopbackNode) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	flags = flags &^ syscall.O_APPEND
	p := n.path()
	f, err := syscall.Open(p, int(flags), 0)
	if err != nil {
		return nil, 0, ToErrno(err)
	}
	lf := NewLoopbackFile(f)
	return lf, 0, 0
}

func (n *loopbackNode) Opendir(ctx context.Context) syscall.Errno {
	fd, err := syscall.Open(n.path(), syscall.O_DIRECTORY, 0755)
	if err != nil {
		return ToErrno(err)
	}
	syscall.Close(fd)
	return OK
}

func (n *loopbackNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	return NewLoopbackDirStream(n.path())
}

func (n *loopbackNode) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		return f.(FileGetattrer).Getattr(ctx, out)
	}

	p := n.path()

	var err error
	st := syscall.Stat_t{}
	if &n.Inode == n.Root() {
		err = syscall.Stat(p, &st)
	} else {
		err = syscall.Lstat(p, &st)
	}

	if err != nil {
		return ToErrno(err)
	}
	out.FromStat(&st)
	return OK
}

var _ = (NodeSetattrer)((*loopbackNode)(nil))

func (n *loopbackNode) Setattr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	p := n.path()
	fsa, ok := f.(FileSetattrer)
	if ok && fsa != nil {
		fsa.Setattr(ctx, in, out)
	} else {
		if m, ok := in.GetMode(); ok {
			if err := syscall.Chmod(p, m); err != nil {
				return ToErrno(err)
			}
		}

		uid, uok := in.GetUID()
		gid, gok := in.GetGID()
		if uok || gok {
			suid := -1
			sgid := -1
			if uok {
				suid = int(uid)
			}
			if gok {
				sgid = int(gid)
			}
			if err := syscall.Chown(p, suid, sgid); err != nil {
				return ToErrno(err)
			}
		}

		mtime, mok := in.GetMTime()
		atime, aok := in.GetATime()

		if mok || aok {

			ap := &atime
			mp := &mtime
			if !aok {
				ap = nil
			}
			if !mok {
				mp = nil
			}
			var ts [2]syscall.Timespec
			ts[0] = fuse.UtimeToTimespec(ap)
			ts[1] = fuse.UtimeToTimespec(mp)

			if err := syscall.UtimesNano(p, ts[:]); err != nil {
				return ToErrno(err)
			}
		}

		if sz, ok := in.GetSize(); ok {
			if err := syscall.Truncate(p, int64(sz)); err != nil {
				return ToErrno(err)
			}
		}
	}

	fga, ok := f.(FileGetattrer)
	if ok && fga != nil {
		fga.Getattr(ctx, out)
	} else {
		st := syscall.Stat_t{}
		err := syscall.Lstat(p, &st)
		if err != nil {
			return ToErrno(err)
		}
		out.FromStat(&st)
	}
	return OK
}

// NewLoopbackRoot returns a root node for a loopback file system whose
// root is at the given root. This node implements all NodeXxxxer
// operations available.
func NewLoopbackRoot(rootPath string) (InodeEmbedder, error) {
	var st syscall.Stat_t
	err := syscall.Stat(rootPath, &st)
	if err != nil {
		return nil, err
	}

	root := &loopbackRoot{
		Path: rootPath,
		Dev:  uint64(st.Dev),
	}

	return root.newNode(), nil
}
