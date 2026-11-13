// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v3/fuse"
	"github.com/hanwen/go-fuse/v3/internal/openat"
	"github.com/hanwen/go-fuse/v3/internal/renameat"
	"golang.org/x/sys/unix"
)

// LoopbackRoot holds the parameters for creating a new loopback
// filesystem. Loopback filesystem delegate their operations to an
// underlying POSIX file system.
type LoopbackRoot struct {
	// The path to the root of the underlying file system.
	Path string

	// The device on which the Path resides. This must be set if
	// the underlying filesystem crosses file systems.
	Dev uint64

	// NewNode returns a new InodeEmbedder to be used to respond
	// to a LOOKUP/CREATE/MKDIR/MKNOD opcode. If not set, use a
	// LoopbackNode.
	//
	// Deprecated: use NodeWrapChilder instead.
	NewNode func(rootData *LoopbackRoot, parent *Inode, name string, st *syscall.Stat_t) InodeEmbedder

	// RootNode is the root of the Loopback. This must be set if
	// the Loopback file system is not the root of the FUSE
	// mount. It is set automatically by NewLoopbackRoot.
	RootNode InodeEmbedder

	rootMu   sync.RWMutex
	rootFile *os.File
}

// withRootFd runs fn with a descriptor for Path, opening it if
// needed. Symlinks in Path itself are followed.
func (r *LoopbackRoot) withRootFd(fn func(fd int) error) error {
	for {
		r.rootMu.RLock()
		if f := r.rootFile; f != nil {
			defer r.rootMu.RUnlock()
			return fn(int(f.Fd()))
		}
		r.rootMu.RUnlock()

		r.rootMu.Lock()
		var err error
		if r.rootFile == nil {
			var fd int
			fd, err = unix.Open(r.Path, dirOpenFlags|unix.O_CLOEXEC, 0)
			if err == nil {
				r.rootFile = os.NewFile(uintptr(fd), r.Path)
			}
		}
		r.rootMu.Unlock()
		if err != nil {
			return err
		}
	}
}

// Close closes the descriptor for Path. It is reopened on the next
// operation that needs it. Open file handles are not affected. Close
// is called when the root LoopbackNode is forgotten, eg. on unmount.
func (r *LoopbackRoot) Close() error {
	r.rootMu.Lock()
	defer r.rootMu.Unlock()
	if r.rootFile == nil {
		return nil
	}
	err := r.rootFile.Close()
	r.rootFile = nil
	return err
}

// openat opens rel beneath the root without following symlinks.
func (r *LoopbackRoot) openat(rel string, flags int, mode uint32) (int, syscall.Errno) {
	if rel == "" {
		rel = "."
	}
	fd := -1
	err := r.withRootFd(func(rootFd int) error {
		var err error
		fd, err = openat.Openat(rootFd, rel, flags, mode)
		return err
	})
	return fd, ToErrno(err)
}

func (r *LoopbackRoot) newNode(parent *Inode, name string, st *syscall.Stat_t) InodeEmbedder {
	if r.NewNode != nil {
		return r.NewNode(r, parent, name, st)
	}
	return &LoopbackNode{
		RootData: r,
	}
}

func genFromBtime(btime *syscall.Timespec) uint64 {
	if btime.Sec == 0 && btime.Nsec == 0 {
		return 1
	}
	return uint64(btime.Sec)*1e9 + uint64(btime.Nsec)
}

func (r *LoopbackRoot) idFromStat(st *syscall.Stat_t, gen uint64) StableAttr {
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
		Gen:  gen,
		// This should work well for traditional backing FSes,
		// not so much for other go-fuse FS-es
		Ino: (swapped ^ swappedRootDev) ^ st.Ino,
	}
}

// LoopbackNode is a filesystem node in a loopback file system. It is
// public so it can be used as a basis for other loopback based
// filesystems. See NewLoopbackFile or LoopbackRoot for more
// information.
type LoopbackNode struct {
	Inode

	// RootData points back to the root of the loopback filesystem.
	RootData *LoopbackRoot
}

// loopbackNodeEmbedder can only be implemented by the LoopbackNode
// concrete type.
type loopbackNodeEmbedder interface {
	loopbackNode() *LoopbackNode
}

func (n *LoopbackNode) loopbackNode() *LoopbackNode {
	return n
}

var _ = (NodeOnForgetter)((*LoopbackNode)(nil))

// OnForget closes the root descriptor when the root node goes away.
// Embedders that override OnForget should call this too.
func (n *LoopbackNode) OnForget() {
	if n.EmbeddedInode() == n.root() {
		n.RootData.Close()
	}
}

var _ = (NodeStatfser)((*LoopbackNode)(nil))

func (n *LoopbackNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	var fd int
	var errno syscall.Errno
	if n.StableAttr().Mode&syscall.S_IFMT == syscall.S_IFDIR {
		fd, errno = n.dirFd()
	} else {
		fd, _, errno = n.parentFd()
	}
	if errno != 0 {
		return errno
	}
	defer syscall.Close(fd)
	s := syscall.Statfs_t{}
	if err := syscall.Fstatfs(fd, &s); err != nil {
		return ToErrno(err)
	}
	out.FromStatfsT(&s)
	return OK
}

// path returns the full path to the file in the underlying file
// system.
func (n *LoopbackNode) root() *Inode {
	var rootNode *Inode
	if n.RootData.RootNode != nil {
		rootNode = n.RootData.RootNode.EmbeddedInode()
	} else {
		rootNode = n.Root()
	}

	return rootNode
}

// relativePath returns the path the node, relative to to the root directory
func (n *LoopbackNode) relativePath() string {
	return n.Path(n.root())
}

// dirFd opens n, which must be a directory.
func (n *LoopbackNode) dirFd() (int, syscall.Errno) {
	return n.RootData.openat(n.relativePath(), dirOpenFlags, 0)
}

// parentFd opens the directory holding n, and returns n's name in
// it. For the root, these are the root itself and ".".
func (n *LoopbackNode) parentFd() (int, string, syscall.Errno) {
	dir, name := "", "."
	if rel := n.relativePath(); rel != "" {
		name = rel
		if i := strings.LastIndexByte(rel, '/'); i >= 0 {
			dir, name = rel[:i], rel[i+1:]
		}
	}
	fd, errno := n.RootData.openat(dir, dirOpenFlags, 0)
	return fd, name, errno
}

var _ = (NodeLookuper)((*LoopbackNode)(nil))

func (n *LoopbackNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)

	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)
	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))
	return ch, 0
}

// preserveOwner sets uid and gid of name in dirfd according to the
// caller information in `ctx`.
func (n *LoopbackNode) preserveOwner(ctx context.Context, dirfd int, name string) error {
	if os.Getuid() != 0 {
		return nil
	}
	caller, ok := fuse.FromContext(ctx)
	if !ok {
		return nil
	}
	return unix.Fchownat(dirfd, name, int(caller.Uid), int(caller.Gid), unix.AT_SYMLINK_NOFOLLOW)
}

var _ = (NodeMknoder)((*LoopbackNode)(nil))

func (n *LoopbackNode) Mknod(ctx context.Context, name string, mode, rdev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)
	if err := n.mknodat(dirfd, name, mode, rdev); err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, dirfd, name)
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		unix.Unlinkat(dirfd, name, 0)
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)

	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))

	return ch, 0
}

var _ = (NodeMkdirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)
	if err := unix.Mkdirat(dirfd, name, mode); err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, dirfd, name)
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR)
		return nil, ToErrno(err)
	}

	out.Attr.FromStat(&st)

	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))

	return ch, 0
}

var _ = (NodeRmdirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)
	return ToErrno(unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR))
}

var _ = (NodeUnlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Unlink(ctx context.Context, name string) syscall.Errno {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)
	return ToErrno(unix.Unlinkat(dirfd, name, 0))
}

var _ = (NodeRenamer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Rename(ctx context.Context, name string, newParent InodeEmbedder, newName string, flags uint32) syscall.Errno {
	e2, ok := newParent.(loopbackNodeEmbedder)
	if !ok {
		return syscall.EXDEV
	}

	if e2.loopbackNode().RootData != n.RootData {
		return syscall.EXDEV
	}

	if flags != 0 {
		return n.rename2(name, e2.loopbackNode(), newName, flags)
	}

	fd1, errno := n.dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(fd1)
	fd2, errno := e2.loopbackNode().dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(fd2)
	return ToErrno(unix.Renameat(fd1, name, fd2, newName))
}

var _ = (NodeCreater)((*LoopbackNode)(nil))

func (n *LoopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *Inode, fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, nil, 0, errno
	}
	defer syscall.Close(dirfd)
	flags = flags &^ syscall.O_APPEND
	fd, err := unix.Openat(dirfd, name, int(flags)|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
	if err != nil {
		return nil, nil, 0, ToErrno(err)
	}
	n.preserveOwner(ctx, dirfd, name)
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := fstatFd(fd, &st, &btime); err != nil {
		syscall.Close(fd)
		return nil, nil, 0, ToErrno(err)
	}

	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))
	lf := NewLoopbackFile(fd)

	out.FromStat(&st)
	return ch, lf, 0, 0
}

func (n *LoopbackNode) rename2(name string, newParent *LoopbackNode, newName string, flags uint32) syscall.Errno {
	fd1, errno := n.dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(fd1)
	fd2, errno := newParent.dirFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(fd2)

	var st syscall.Stat_t
	if err := syscall.Fstat(fd1, &st); err != nil {
		return ToErrno(err)
	}

	// Double check that nodes didn't change from under us. Gen is
	// irrelevant here, since only Ino is compared.
	if n.root() != n.EmbeddedInode() && n.Inode.StableAttr().Ino != n.RootData.idFromStat(&st, 0).Ino {
		return syscall.EBUSY
	}
	if err := syscall.Fstat(fd2, &st); err != nil {
		return ToErrno(err)
	}

	if (newParent.root() != newParent.EmbeddedInode()) && newParent.Inode.StableAttr().Ino != n.RootData.idFromStat(&st, 0).Ino {
		return syscall.EBUSY
	}

	return ToErrno(renameat.Renameat(fd1, name, fd2, newName, uint(flags)))
}

var _ = (NodeSymlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)
	if err := unix.Symlinkat(target, dirfd, name); err != nil {
		return nil, ToErrno(err)
	}
	n.preserveOwner(ctx, dirfd, name)
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		unix.Unlinkat(dirfd, name, 0)
		return nil, ToErrno(err)
	}
	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))

	out.Attr.FromStat(&st)
	return ch, 0
}

var _ = (NodeLinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Link(ctx context.Context, target InodeEmbedder, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	e2, ok := target.(loopbackNodeEmbedder)
	if !ok {
		return nil, syscall.EXDEV
	}

	if e2.loopbackNode().RootData != n.RootData {
		return nil, syscall.EXDEV
	}

	// The target is in no directory, so the loopback has no path to link it from. A file created with O_TMPFILE is
	// named only by its descriptor, and LINK does not carry one.
	if _, parent := target.EmbeddedInode().Parent(); parent == nil {
		return nil, syscall.EXDEV
	}

	tfd, tname, errno := e2.loopbackNode().parentFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(tfd)
	dirfd, errno := n.dirFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)
	if err := unix.Linkat(tfd, tname, dirfd, name, 0); err != nil {
		return nil, ToErrno(err)
	}
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		unix.Unlinkat(dirfd, name, 0)
		return nil, ToErrno(err)
	}
	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st, genFromBtime(&btime)))

	out.Attr.FromStat(&st)
	return ch, 0
}

var _ = (NodeReadlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return nil, errno
	}
	defer syscall.Close(dirfd)

	for l := 256; ; l *= 2 {
		buf := make([]byte, l)
		sz, err := unix.Readlinkat(dirfd, name, buf)
		if err != nil {
			return nil, ToErrno(err)
		}

		if sz < len(buf) {
			return buf[:sz], 0
		}
	}
}

var _ = (NodeOpener)((*LoopbackNode)(nil))

func (n *LoopbackNode) Open(ctx context.Context, flags uint32) (fh FileHandle, fuseFlags uint32, errno syscall.Errno) {
	flags = flags &^ (syscall.O_APPEND | fuse.FMODE_EXEC)

	fd, errno := n.RootData.openat(n.relativePath(), int(flags), 0)
	if errno != 0 {
		return nil, 0, errno
	}
	lf := NewLoopbackFile(fd)
	return lf, 0, 0
}

var _ = (NodeOpendirHandler)((*LoopbackNode)(nil))

func (n *LoopbackNode) OpendirHandle(ctx context.Context, flags uint32) (FileHandle, uint32, syscall.Errno) {
	ds, errno := n.Readdir(ctx)
	if errno != 0 {
		return nil, 0, errno
	}
	return ds, 0, errno
}

var _ = (NodeReaddirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Readdir(ctx context.Context) (DirStream, syscall.Errno) {
	fd, errno := n.RootData.openat(n.relativePath(), syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if errno != 0 {
		return nil, errno
	}
	return NewLoopbackDirStreamFd(fd)
}

var _ = (NodeGetattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getattr(ctx context.Context, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		if fga, ok := f.(FileGetattrer); ok {
			return fga.Getattr(ctx, out)
		}
	}

	dirfd, name, errno := n.parentFd()
	if errno != 0 {
		return errno
	}
	defer syscall.Close(dirfd)

	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := statAt(dirfd, name, &st, &btime); err != nil {
		return ToErrno(err)
	}
	out.FromStat(&st)
	return OK
}

var _ = (NodeSetattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setattr(ctx context.Context, f FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fsa, _ := f.(FileSetattrer)
	fga, _ := f.(FileGetattrer)
	dirfd, name := -1, ""
	if fsa == nil || fga == nil {
		var errno syscall.Errno
		dirfd, name, errno = n.parentFd()
		if errno != 0 {
			return errno
		}
		defer syscall.Close(dirfd)
	}

	if fsa != nil {
		if errno := fsa.Setattr(ctx, in, out); errno != 0 {
			return errno
		}
	} else {
		if m, ok := in.GetMode(); ok {
			if err := fchmodatNoFollow(dirfd, name, m); err != nil {
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
			if err := unix.Fchownat(dirfd, name, suid, sgid, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return ToErrno(err)
			}
		}

		// Truncate before setting times, so an explicit mtime is
		// not clobbered by the truncate.
		if sz, ok := in.GetSize(); ok {
			fd, err := unix.Openat(dirfd, name, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return ToErrno(err)
			}
			err = unix.Ftruncate(fd, int64(sz))
			syscall.Close(fd)
			if err != nil {
				return ToErrno(err)
			}
		}

		mtime, mok := in.GetMTime()
		atime, aok := in.GetATime()

		if mok || aok {
			ta := unix.Timespec{Nsec: unix_UTIME_OMIT}
			tm := unix.Timespec{Nsec: unix_UTIME_OMIT}
			var err error
			if aok {
				ta, err = unix.TimeToTimespec(atime)
				if err != nil {
					return ToErrno(err)
				}
			}
			if mok {
				tm, err = unix.TimeToTimespec(mtime)
				if err != nil {
					return ToErrno(err)
				}
			}
			ts := []unix.Timespec{ta, tm}
			if err := unix.UtimesNanoAt(dirfd, name, ts, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return ToErrno(err)
			}
		}
	}

	if fga != nil {
		fga.Getattr(ctx, out)
	} else {
		var st syscall.Stat_t
		var btime syscall.Timespec
		if err := statAt(dirfd, name, &st, &btime); err != nil {
			return ToErrno(err)
		}
		out.FromStat(&st)
	}
	return OK
}

var _ = (NodeCopyFileRanger)((*LoopbackNode)(nil))

func (n *LoopbackNode) CopyFileRange(ctx context.Context, fhIn FileHandle,
	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
	len uint64, flags uint64) (count uint32, errno syscall.Errno) {
	lfIn, ok := fhIn.(*LoopbackFile)
	if !ok {
		return 0, unix.ENOTSUP
	}
	lfOut, ok := fhOut.(*LoopbackFile)
	if !ok {
		return 0, unix.ENOTSUP
	}
	signedOffIn := int64(offIn)
	signedOffOut := int64(offOut)
	lfIn.withFd(func(fdIn int) syscall.Errno {
		return lfOut.withFd(func(fdOut int) syscall.Errno {
			count, errno = doCopyFileRange(fdIn, signedOffIn, fdOut, signedOffOut, int(len), int(flags))
			return OK
		})
	})
	return count, errno
}

// NewLoopbackRoot returns a root node for a loopback file system whose
// root is at the given root. This node implements all NodeXxxxer
// operations available.
func NewLoopbackRoot(rootPath string) (InodeEmbedder, error) {
	root := &LoopbackRoot{
		Path: rootPath,
	}
	var st syscall.Stat_t
	var btime syscall.Timespec
	if err := root.withRootFd(func(fd int) error {
		return fstatFd(fd, &st, &btime)
	}); err != nil {
		return nil, err
	}
	root.Dev = uint64(st.Dev)

	rootNode := root.newNode(nil, "", &st)
	root.RootNode = rootNode
	return rootNode, nil
}
