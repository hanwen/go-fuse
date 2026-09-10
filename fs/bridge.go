// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/internal"
)

func errnoToStatus(errno syscall.Errno) fuse.Status {
	return fuse.Status(errno)
}

type fileEntry struct {
	file FileHandle

	// inode is the *Inode this handle was opened against, captured at
	// registerFile time. It may differ from the node currently
	// registered under this handle's nodeid (nodeEntry.inode) if the
	// nodeid was reused for a new incarnation while this handle was
	// still open - RELEASE must dispatch against inode, not the
	// nodeEntry's current state.
	inode *Inode

	// index into nodeEntry.openFiles
	nodeIndex int

	// Handle number which we communicate to the kernel.
	fh uint32

	// Protects directory fields. Must be acquired before rawBridge.ids'
	// and rawBridge.backingMu's locks.
	mu sync.Mutex

	// Directory
	hasOverflow   bool
	overflow      fuse.DirEntry
	overflowErrno syscall.Errno

	// Store the last read, in case readdir was interrupted.
	lastRead []fuse.DirEntry

	// dirOffset is the current location in the directory (see `telldir(3)`).
	// The value is equivalent to `d_off` (see `getdents(2)`) of the last
	// directory entry sent to the kernel so far.
	// If `dirOffset` and `fuse.DirEntryList.offset` disagree, then a
	// directory seek has taken place.
	dirOffset uint64

	// We try to associate a file for stat() calls, but the kernel
	// can issue a RELEASE and GETATTR in parallel. This waitgroup
	// avoids that the RELEASE will invalidate the file descriptor
	// before we finish processing GETATTR.
	wg sync.WaitGroup
}

// ServerCallbacks are calls into the kernel to manipulate the inode,
// entry and page cache.  They are stubbed so filesystems can be
// unittested without mounting them.
type ServerCallbacks interface {
	DeleteNotify(parent uint64, child uint64, name string) fuse.Status
	EntryNotify(parent uint64, name string) fuse.Status
	InodeNotify(node uint64, off int64, length int64) fuse.Status
	InodeRetrieveCache(node uint64, offset int64, dest []byte) (n int, st fuse.Status)
	InodeNotifyStoreCache(node uint64, offset int64, data []byte) fuse.Status
}

// TODO: fold serverBackingFdCallbacks into ServerCallbacks and bump API version
type serverBackingFdCallbacks interface {
	RegisterBackingFd(*fuse.BackingMap) (int32, syscall.Errno)
	UnregisterBackingFd(id int32) syscall.Errno
}

type rawBridge struct {
	options Options
	root    *Inode
	server  ServerCallbacks

	// ids owns the nodeid <-> Inode and file handle <-> fileEntry
	// tables, including each node's openFiles and backing-fd state
	// (see identitytable.go's nodeEntry). rawBridge is only ever used
	// through a pointer (see NewNodeFS), so holding it by value here
	// adds no extra indirection or allocation.
	ids mapIdentityTable

	// backingMu protects the backingID/backingIDRefcount fields of the
	// nodeEntry values ids hands out (see identitytable.go). It is
	// separate from ids' own mutex because addBackingID/
	// releaseBackingIDRef hold it across a call into ServerCallbacks.
	// Locks for inodes must be taken before backingMu.
	backingMu sync.Mutex

	// If set, don't try to register backing file for Create/Open calls.
	disableBackingFiles bool
}

// newInode creates creates new inode pointing to ops.
func (b *rawBridge) newInodeUnlocked(ops InodeEmbedder, id StableAttr, persistent bool) *Inode {
	if id.Reserved() {
		log.Panicf("using reserved ID %d for inode number", id.Ino)
	}

	// This ops already was populated. Just return it.
	if ops.embed().bridge != nil {
		return ops.embed()
	}

	// Only the file type bits matter
	id.Mode = id.Mode & syscall.S_IFMT
	if id.Mode == 0 {
		id.Mode = fuse.S_IFREG
	}

	if id.Ino == 0 {
		// Find free inode number.
		for {
			id.Ino = b.ids.nextAutomaticIno()
			if id.Ino == fuse.FUSE_ROOT_ID && b.options.ExternalNodeID {
				continue
			}
			if id.Ino == 0 || id.Ino == ^uint64(0) {
				continue
			}
			if b.ids.findByAttr(id) == nil {
				break
			}
		}
	}

	initInode(ops.embed(), ops, id, b, persistent, b.ids.allocateNodeID())
	return ops.embed()
}

func (b *rawBridge) logf(format string, args ...any) {
	if b.options.Logger != nil {
		b.options.Logger.Printf(format, args...)
	}
}

func (b *rawBridge) newInode(ctx context.Context, ops InodeEmbedder, id StableAttr, persistent bool) *Inode {
	ch := b.newInodeUnlocked(ops, id, persistent)
	if ch != ops.embed() {
		return ch
	}

	if oa, ok := ops.(NodeOnAdder); ok {
		oa.OnAdd(ctx)
	}
	return ch
}

// addNewChild inserts the child into the tree. Returns file handle if file != nil.
// Unless fileFlags has the syscall.O_EXCL bit set, child.stableAttr will be used
// to find an already-known node. If one is found, `child` is ignored and the
// already-known one is used. The node that was actually used is returned.
func (b *rawBridge) addNewChild(parent *Inode, name string, child *Inode, file FileHandle, fileFlags uint32, out *fuse.EntryOut) (selected *Inode, fe *fileEntry, entry *nodeEntry) {
	if name == "." || name == ".." {
		log.Panicf("BUG: tried to add virtual entry %q to the actual tree", name)
	}

	// the same node can be looked up through 2 paths in parallel, eg.
	//
	//	    root
	//	    /  \
	//	  dir1 dir2
	//	    \  /
	//	    file
	//
	// dir1.Lookup("file") and dir2.Lookup("file") are executed
	// simultaneously.  The matching StableAttrs ensure that we return the
	// same node.
	id := child.stableAttr
	if id.Mode & ^(uint32(syscall.S_IFMT)) != 0 {
		log.Panicf("%#v", id)
	}
	exclusive := fileFlags&syscall.O_EXCL != 0
	for {
		lockNodes(parent, child)
		// registerNew atomically re-checks for an existing node under
		// id and registers child if it wins. If it lost the race
		// (someone else registered a different node under id between
		// our last check and now), retry with that node instead - it
		// may be the node we should use, or the race may repeat.
		winner, winnerEntry := b.ids.registerNew(id, child, exclusive)
		if winner == child {
			entry = winnerEntry
			break
		}
		unlockNodes(parent, child)
		child = winner
	}

	child.lookupCount++
	child.changeCounter++

	if file != nil {
		fe = b.ids.registerFile(entry, file, fileFlags)
	}

	parent.setEntry(name, child)

	out.NodeId = child.nodeId
	out.Generation = child.stableAttr.Gen
	out.Attr.Ino = child.stableAttr.Ino

	unlockNodes(parent, child)

	return child, fe, entry
}

func (b *rawBridge) setEntryOutTimeout(out *fuse.EntryOut) {
	b.setAttr(&out.Attr)
	if b.options.AttrTimeout != nil && out.AttrTimeout() == 0 {
		out.SetAttrTimeout(*b.options.AttrTimeout)
	}
	if b.options.EntryTimeout != nil && out.EntryTimeout() == 0 {
		out.SetEntryTimeout(*b.options.EntryTimeout)
	}
}

func (b *rawBridge) setAttr(out *fuse.Attr) {
	if !b.options.NullPermissions && out.Mode&07777 == 0 {
		out.Mode |= 0644
		if out.Mode&syscall.S_IFDIR != 0 {
			out.Mode |= 0111
		}
	}
	if b.options.UID != 0 && out.Uid == 0 {
		out.Uid = b.options.UID
	}
	if b.options.GID != 0 && out.Gid == 0 {
		out.Gid = b.options.GID
	}
	setBlocks(out)
}

func (b *rawBridge) setAttrTimeout(out *fuse.AttrOut) {
	if b.options.AttrTimeout != nil && out.Timeout() == 0 {
		out.SetTimeout(*b.options.AttrTimeout)
	}
}

// NewNodeFS creates a node based filesystem based on the
// InodeEmbedder instance for the root of the tree.
// If nil is given as opts, default settings are
// applied, which are 1 second entry and attribute timeout.
func NewNodeFS(root InodeEmbedder, opts *Options) fuse.RawFileSystem {
	if opts == nil {
		oneSec := time.Second
		opts = &Options{
			EntryTimeout: &oneSec,
			AttrTimeout:  &oneSec,
		}
	}
	bridge := &rawBridge{
		server:  opts.ServerCallbacks,
		options: *opts,
	}
	bridge.ids.initIdentityTable(opts.FirstAutomaticIno)

	stableAttr := StableAttr{
		Ino:  root.embed().StableAttr().Ino,
		Mode: fuse.S_IFDIR,
	}
	if opts.RootStableAttr != nil {
		stableAttr.Ino = opts.RootStableAttr.Ino
		stableAttr.Gen = opts.RootStableAttr.Gen
	}

	initInode(root.embed(), root,
		stableAttr,
		bridge,
		false,
		1,
	)
	bridge.root = root.embed()
	bridge.root.lookupCount = 1
	bridge.ids.registerRoot(bridge.root)

	if opts.OnAdd != nil {
		opts.OnAdd(context.Background())
	} else if oa, ok := root.(NodeOnAdder); ok {
		oa.OnAdd(context.Background())
	}

	return bridge
}

func (b *rawBridge) String() string {
	return "rawBridge"
}

// entry resolves a (nodeid, fh) pair as sent by the kernel, panicking
// if nodeID is unknown. Prefer inode() when only the *Inode is needed;
// use entry() when the caller also wants to reuse the *nodeEntry (eg.
// for registerFile/addBackingID) without a second lookup.
func (b *rawBridge) entry(nodeID uint64, fh uint64) (*nodeEntry, *fileEntry) {
	e, f := b.ids.node(nodeID, fh)
	if e == nil {
		log.Panicf("unknown node %d", nodeID)
	}
	return e, f
}

func (b *rawBridge) inode(id uint64, fh uint64) (*Inode, *fileEntry) {
	e, f := b.entry(id, fh)
	return e.inode, f
}

func (b *rawBridge) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	ctx := &fuse.Context{Caller: header.Caller, Cancel: cancel}
	child, errno := b.lookup(ctx, parent, name, out)

	if errno != 0 {
		if errno == syscall.ENOENT && b.options.NegativeTimeout != nil && out.EntryTimeout() == 0 {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
			errno = 0
		}
		return errnoToStatus(errno)
	}

	child, _, _ = b.addNewChild(parent, name, child, nil, 0, out)
	child.setEntryOut(out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) lookup(ctx *fuse.Context, parent *Inode, name string, out *fuse.EntryOut) (*Inode, syscall.Errno) {
	if lu, ok := parent.ops.(NodeLookuper); ok {
		return lu.Lookup(ctx, name, out)
	}

	child := parent.GetChild(name)
	if child == nil {
		return nil, syscall.ENOENT
	}

	if ga, ok := child.ops.(NodeGetattrer); ok {
		var a fuse.AttrOut
		errno := ga.Getattr(ctx, nil, &a)
		if errno == 0 {
			out.Attr = a.Attr
		}
	}

	return child, OK
}

func (b *rawBridge) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var errno syscall.Errno
	if mops, ok := parent.ops.(NodeRmdirer); ok {
		errno = mops.Rmdir(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	// TODO - this should not succeed silently.

	if errno == 0 {
		parent.RmChild(name)
	}
	return errnoToStatus(errno)
}

func (b *rawBridge) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var errno syscall.Errno
	if mops, ok := parent.ops.(NodeUnlinker); ok {
		errno = mops.Unlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	// TODO - this should not succeed silently.

	if errno == 0 {
		parent.RmChild(name)
	}
	return errnoToStatus(errno)
}

func (b *rawBridge) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	mops, ok := parent.ops.(NodeMkdirer)
	if !ok {
		return fuse.ENOTSUP
	}
	child, errno := mops.Mkdir(ctx, name, input.Mode, out)

	if errno != 0 {
		return errnoToStatus(errno)
	}

	if out.Attr.Mode&^07777 == 0 {
		out.Attr.Mode |= fuse.S_IFDIR
	}

	if out.Attr.Mode&^07777 != fuse.S_IFDIR {
		log.Panicf("Mkdir: mode must be S_IFDIR (%o), got %o", fuse.S_IFDIR, out.Attr.Mode)
	}

	child, _, _ = b.addNewChild(parent, name, child, nil, syscall.O_EXCL, out)
	child.setEntryOut(out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	mops, ok := parent.ops.(NodeMknoder)
	if !ok {
		return fuse.ENOTSUP
	}
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	child, errno := mops.Mknod(ctx, name, input.Mode, input.Rdev, out)
	if errno != 0 {
		return errnoToStatus(errno)
	}

	child, _, _ = b.addNewChild(parent, name, child, nil, syscall.O_EXCL, out)
	child.setEntryOut(out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	mops, ok := parent.ops.(NodeCreater)
	if !ok {
		return fuse.EROFS
	}
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	child, f, flags, errno := mops.Create(ctx, name, input.Flags, input.Mode, &out.EntryOut)

	if errno != 0 {
		return errnoToStatus(errno)
	}

	child, fe, entry := b.addNewChild(parent, name, child, f, input.Flags|syscall.O_CREAT|syscall.O_EXCL, &out.EntryOut)
	if fe != nil {
		out.Fh = uint64(fe.fh)
	}
	out.OpenFlags = flags

	b.backingMu.Lock()
	b.addBackingID(entry, f, &out.OpenOut)
	b.backingMu.Unlock()
	child.setEntryOut(&out.EntryOut)
	b.setEntryOutTimeout(&out.EntryOut)
	return fuse.OK
}

func (b *rawBridge) Forget(nodeid, nlookup uint64) {
	n, _ := b.inode(nodeid, 0)
	hasLookups, _, _ := n.removeRef(nlookup, false)

	if !hasLookups {
		b.ids.compact()
	}
}

func (b *rawBridge) SetDebug(debug bool) {}

func (b *rawBridge) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	e, fEntry := b.entry(input.NodeId, input.Fh())
	n := e.inode
	f := fEntry.file
	if f == nil {
		// The linux kernel doesnt pass along the file
		// descriptor, so we have to fake it here.
		// See https://github.com/libfuse/libfuse/issues/62
		var done func()
		f, done = b.ids.firstOpenFile(e)
		defer done()
	}
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	return errnoToStatus(b.getattr(ctx, n, f, out))
}

func (b *rawBridge) getattr(ctx context.Context, n *Inode, f FileHandle, out *fuse.AttrOut) syscall.Errno {
	var errno syscall.Errno

	if nodeOps, ok := n.ops.(NodeGetattrer); ok {
		errno = nodeOps.Getattr(ctx, f, out)
	} else if fileOps, ok := f.(FileGetattrer); ok {
		errno = fileOps.Getattr(ctx, out)
	} else {
		// We set Mode below, which is the minimum for success
	}

	if errno == 0 {
		if out.Ino != 0 && n.stableAttr.Ino > 1 && out.Ino != n.stableAttr.Ino {
			b.logf("warning: rawBridge.getattr: overriding ino %d with %d", out.Ino, n.stableAttr.Ino)
		}
		out.Ino = n.stableAttr.Ino
		out.Mode = (out.Attr.Mode & 07777) | n.stableAttr.Mode
		b.setAttr(&out.Attr)
		b.setAttrTimeout(out)
	}
	return errno
}

func (b *rawBridge) SetAttr(cancel <-chan struct{}, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}

	fh, _ := in.GetFh()

	n, fEntry := b.inode(in.NodeId, fh)
	f := fEntry.file

	var errno = syscall.ENOTSUP
	if fops, ok := n.ops.(NodeSetattrer); ok {
		errno = fops.Setattr(ctx, f, in, out)
	} else if fops, ok := f.(FileSetattrer); ok {
		errno = fops.Setattr(ctx, in, out)
	}

	out.Mode = n.stableAttr.Mode | (out.Mode & 07777)
	return errnoToStatus(errno)
}

func (b *rawBridge) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) fuse.Status {
	p1, _ := b.inode(input.NodeId, 0)
	p2, _ := b.inode(input.Newdir, 0)

	if mops, ok := p1.ops.(NodeRenamer); ok {
		errno := mops.Rename(&fuse.Context{Caller: input.Caller, Cancel: cancel}, oldName, p2.ops, newName, input.Flags)
		if errno == 0 {
			if input.Flags&RENAME_EXCHANGE != 0 {
				p1.ExchangeChild(oldName, p2, newName)
			} else {
				// MvChild cannot fail with overwrite=true.
				_ = p1.MvChild(oldName, p2, newName, true)
			}
		}
		return errnoToStatus(errno)
	}
	return fuse.ENOTSUP
}

func (b *rawBridge) Link(cancel <-chan struct{}, input *fuse.LinkIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)
	target, _ := b.inode(input.Oldnodeid, 0)

	mops, ok := parent.ops.(NodeLinker)
	if !ok {
		return fuse.ENOTSUP
	}

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	child, errno := mops.Link(ctx, target.ops, name, out)
	if errno != 0 {
		return errnoToStatus(errno)
	}

	child, _, _ = b.addNewChild(parent, name, child, nil, 0, out)
	child.setEntryOut(out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Symlink(cancel <-chan struct{}, header *fuse.InHeader, target string, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)

	mops, ok := parent.ops.(NodeSymlinker)
	if !ok {
		return fuse.ENOTSUP
	}
	ctx := &fuse.Context{Caller: header.Caller, Cancel: cancel}
	child, status := mops.Symlink(ctx, target, name, out)
	if status != 0 {
		return errnoToStatus(status)
	}

	child, _, _ = b.addNewChild(parent, name, child, nil, syscall.O_EXCL, out)
	child.setEntryOut(out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)

	linker, ok := n.ops.(NodeReadlinker)
	if !ok {
		return nil, fuse.ENOTSUP
	}
	ctx := &fuse.Context{Caller: header.Caller, Cancel: cancel}
	result, errno := linker.Readlink(ctx)
	if errno != 0 {
		return nil, errnoToStatus(errno)
	}

	return result, fuse.OK
}

func (b *rawBridge) Access(cancel <-chan struct{}, input *fuse.AccessIn) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if a, ok := n.ops.(NodeAccesser); ok {
		return errnoToStatus(a.Access(ctx, input.Mask))
	}

	// default: check attributes.
	caller := input.Caller

	var out fuse.AttrOut
	if s := b.getattr(ctx, n, nil, &out); s != 0 {
		return errnoToStatus(s)
	}

	if !internal.HasAccess(caller.Uid, caller.Gid, out.Uid, out.Gid, out.Mode, input.Mask) {
		return fuse.EACCES
	}
	return fuse.OK
}

// Extended attributes.

func (b *rawBridge) GetXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string, data []byte) (uint32, fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)

	if xops, ok := n.ops.(NodeGetxattrer); ok {
		nb, errno := xops.Getxattr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr, data)
		return nb, errnoToStatus(errno)
	}

	return 0, fuse.ENOATTR
}

func (b *rawBridge) ListXAttr(cancel <-chan struct{}, header *fuse.InHeader, dest []byte) (sz uint32, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(NodeListxattrer); ok {
		sz, errno := xops.Listxattr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, dest)
		return sz, errnoToStatus(errno)
	}
	return 0, fuse.OK
}

func (b *rawBridge) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	if xops, ok := n.ops.(NodeSetxattrer); ok {
		return errnoToStatus(xops.Setxattr(&fuse.Context{Caller: input.Caller, Cancel: cancel}, attr, data, input.Flags))
	}
	return fuse.ENOATTR
}

func (b *rawBridge) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) fuse.Status {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(NodeRemovexattrer); ok {
		return errnoToStatus(xops.Removexattr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr))
	}
	return fuse.ENOATTR
}

func (b *rawBridge) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	e, _ := b.entry(input.NodeId, 0)
	n := e.inode

	op, ok := n.ops.(NodeOpener)
	if !ok {
		return fuse.ENOTSUP
	}
	f, flags, errno := op.Open(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Flags)
	if errno != 0 {
		return errnoToStatus(errno)
	}
	out.OpenFlags = flags

	if f != nil {
		fe := b.ids.registerFile(e, f, input.Flags)
		out.Fh = uint64(fe.fh)

		b.backingMu.Lock()
		defer b.backingMu.Unlock()
		b.addBackingID(e, f, out)
	}
	return fuse.OK
}

// must hold backingMu
func (b *rawBridge) addBackingID(e *nodeEntry, f FileHandle, out *fuse.OpenOut) {
	if b.disableBackingFiles {
		return
	}

	bc, ok := b.server.(serverBackingFdCallbacks)
	if !ok {
		b.disableBackingFiles = true
		return
	}
	pth, ok := f.(FilePassthroughFder)
	if !ok {
		return
	}

	if e.backingID == 0 {
		fd, ok := pth.PassthroughFd()
		if !ok {
			return
		}
		m := fuse.BackingMap{
			Fd: int32(fd),
		}
		id, errno := bc.RegisterBackingFd(&m)
		if errno != 0 {
			// This happens if we're not root or CAP_PASSTHROUGH is missing.
			b.disableBackingFiles = true
		} else {
			e.backingID = id
		}
	}

	if e.backingID != 0 {
		out.BackingID = e.backingID
		out.OpenFlags |= fuse.FOPEN_PASSTHROUGH
		out.OpenFlags &= ^uint32(fuse.FOPEN_KEEP_CACHE)
		e.backingIDRefcount++
	}
}

// must hold backingMu
func (b *rawBridge) releaseBackingIDRef(e *nodeEntry) {
	if e.backingID == 0 {
		return
	}

	e.backingIDRefcount--
	if e.backingIDRefcount == 0 {
		errno := b.server.(serverBackingFdCallbacks).UnregisterBackingFd(e.backingID)
		if errno != 0 {
			b.logf("UnregisterBackingFd: %v", errno)
		}
		e.backingID = 0
		e.backingIDRefcount = 0
	} else if e.backingIDRefcount < 0 {
		log.Panic("backingIDRefcount underflow")
	}
}

func (b *rawBridge) Read(cancel <-chan struct{}, input *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if fops, ok := n.ops.(NodeReader); ok {
		res, errno := fops.Read(ctx, f.file, buf, int64(input.Offset))
		return res, errnoToStatus(errno)
	}
	if fr, ok := f.file.(FileReader); ok {
		res, errno := fr.Read(ctx, buf, int64(input.Offset))
		return res, errnoToStatus(errno)
	}

	return nil, fuse.ENOTSUP
}

func (b *rawBridge) GetLk(cancel <-chan struct{}, input *fuse.LkIn, out *fuse.LkOut) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if lops, ok := n.ops.(NodeGetlker); ok {
		return errnoToStatus(lops.Getlk(ctx, f.file, input.Owner, &input.Lk, input.LkFlags, &out.Lk))
	}
	if gl, ok := f.file.(FileGetlker); ok {
		return errnoToStatus(gl.Getlk(ctx, input.Owner, &input.Lk, input.LkFlags, &out.Lk))
	}
	return fuse.ENOTSUP
}

func (b *rawBridge) SetLk(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if lops, ok := n.ops.(NodeSetlker); ok {
		return errnoToStatus(lops.Setlk(ctx, f.file, input.Owner, &input.Lk, input.LkFlags))
	}
	if sl, ok := f.file.(FileSetlker); ok {
		return errnoToStatus(sl.Setlk(ctx, input.Owner, &input.Lk, input.LkFlags))
	}
	return fuse.ENOTSUP
}
func (b *rawBridge) SetLkw(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if lops, ok := n.ops.(NodeSetlkwer); ok {
		return errnoToStatus(lops.Setlkw(ctx, f.file, input.Owner, &input.Lk, input.LkFlags))
	}
	if sl, ok := f.file.(FileSetlkwer); ok {
		return errnoToStatus(sl.Setlkw(ctx, input.Owner, &input.Lk, input.LkFlags))
	}
	return fuse.ENOTSUP
}

func (b *rawBridge) Release(cancel <-chan struct{}, input *fuse.ReleaseIn) {
	e, f := b.releaseFileEntry(input.NodeId, input.Fh)
	if f == nil {
		return
	}
	n := f.inode

	f.wg.Wait()

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if r, ok := n.ops.(NodeReleaser); ok {
		r.Release(ctx, f.file)
	} else if r, ok := f.file.(FileReleaser); ok {
		r.Release(ctx)
	}

	b.backingMu.Lock()
	defer b.backingMu.Unlock()

	b.releaseBackingIDRef(e)
	b.ids.recycleFile(uint32(input.Fh))
}

func (b *rawBridge) ReleaseDir(input *fuse.ReleaseIn) {
	e, f := b.releaseFileEntry(input.NodeId, input.Fh)
	if f == nil {
		return
	}
	f.wg.Wait()

	if frd, ok := f.file.(FileReleasedirer); ok {
		frd.Releasedir(context.Background(), input.ReleaseFlags)
	}

	b.backingMu.Lock()
	defer b.backingMu.Unlock()
	b.releaseBackingIDRef(e)
	b.ids.recycleFile(uint32(input.Fh))
}

func (b *rawBridge) releaseFileEntry(nid uint64, fh uint64) (*nodeEntry, *fileEntry) {
	return b.ids.detachFile(nid, fh)
}

func (b *rawBridge) Write(cancel <-chan struct{}, input *fuse.WriteIn, data []byte) (written uint32, status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if wr, ok := n.ops.(NodeWriter); ok {
		w, errno := wr.Write(ctx, f.file, data, int64(input.Offset))
		return w, errnoToStatus(errno)
	}
	if fr, ok := f.file.(FileWriter); ok {
		w, errno := fr.Write(ctx, data, int64(input.Offset))
		return w, errnoToStatus(errno)
	}

	return 0, fuse.ENOTSUP
}

func (b *rawBridge) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if fl, ok := n.ops.(NodeFlusher); ok {
		return errnoToStatus(fl.Flush(ctx, f.file))
	}
	if fl, ok := f.file.(FileFlusher); ok {
		return errnoToStatus(fl.Flush(ctx))
	}
	return 0
}

func (b *rawBridge) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if fs, ok := n.ops.(NodeFsyncer); ok {
		return errnoToStatus(fs.Fsync(ctx, f.file, input.FsyncFlags))
	}
	if fs, ok := f.file.(FileFsyncer); ok {
		return errnoToStatus(fs.Fsync(ctx, input.FsyncFlags))
	}
	return fuse.ENOTSUP
}

func (b *rawBridge) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if a, ok := n.ops.(NodeAllocater); ok {
		return errnoToStatus(a.Allocate(ctx, f.file, input.Offset, input.Length, input.Mode))
	}
	if a, ok := f.file.(FileAllocater); ok {
		return errnoToStatus(a.Allocate(ctx, input.Offset, input.Length, input.Mode))
	}
	return fuse.ENOTSUP
}

func (b *rawBridge) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	e, _ := b.entry(input.NodeId, 0)
	n := e.inode

	var fh FileHandle
	var fuseFlags uint32
	var errno syscall.Errno

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}

	nod, _ := n.ops.(NodeOpendirer)
	nrd, _ := n.ops.(NodeReaddirer)

	if odh, ok := n.ops.(NodeOpendirHandler); ok {
		fh, fuseFlags, errno = odh.OpendirHandle(ctx, input.Flags)

		if errno != 0 {
			return errnoToStatus(errno)
		}
	} else {
		if nod != nil {
			errno = nod.Opendir(ctx)
			if errno != 0 {
				return errnoToStatus(errno)
			}
		}

		var ctor func(context.Context) (DirStream, syscall.Errno)
		if nrd != nil {
			ctor = func(ctx context.Context) (DirStream, syscall.Errno) {
				return nrd.Readdir(ctx)
			}
		} else {
			ctor = func(ctx context.Context) (DirStream, syscall.Errno) {
				return n.childrenAsDirstream(), 0
			}
		}
		fh = &dirStreamAsFile{creator: ctor}
	}

	if fuseFlags&(fuse.FOPEN_CACHE_DIR|fuse.FOPEN_KEEP_CACHE) != 0 {
		fuseFlags |= fuse.FOPEN_CACHE_DIR | fuse.FOPEN_KEEP_CACHE
	}
	fe := b.ids.registerFile(e, fh, 0)
	out.Fh = uint64(fe.fh)
	out.OpenFlags = fuseFlags
	return fuse.OK
}

func (n *Inode) childrenAsDirstream() DirStream {
	lst := n.childrenList()
	r := make([]fuse.DirEntry, 0, len(lst))
	for _, e := range lst {
		r = append(r, fuse.DirEntry{Mode: e.Inode.Mode(),
			Name: e.Name,
			Ino:  e.Inode.StableAttr().Ino})
	}
	return NewListDirStream(r)
}

func (b *rawBridge) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return b.readDirMaybeLookup(cancel, input, out, true)
}

func (b *rawBridge) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return b.readDirMaybeLookup(cancel, input, out, false)
}

func (b *rawBridge) readDirMaybeLookup(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList, lookup bool) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	direnter, ok := f.file.(FileReaddirenter)
	if !ok {
		return fuse.OK
	}
	getdent := direnter.Readdirent

	f.mu.Lock()
	defer f.mu.Unlock()

	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	interruptedRead := false
	if input.Offset != f.dirOffset {
		// If the last readdir(plus) was interrupted, the
		// kernel may consume just one entry from the readdir,
		// and redo it.
		for i, e := range f.lastRead {
			if e.Off == input.Offset {
				interruptedRead = true
				todo := f.lastRead[i+1:]
				todo = make([]fuse.DirEntry, len(todo))
				copy(todo, f.lastRead[i+1:])
				getdent = func(context.Context) (*fuse.DirEntry, syscall.Errno) {
					if len(todo) > 0 {
						de := &todo[0]
						todo = todo[1:]
						return de, 0
					}
					return nil, 0
				}
				f.dirOffset = input.Offset
				break
			}
		}
	}

	if input.Offset != f.dirOffset {
		if sd, ok := f.file.(FileSeekdirer); ok {
			errno := sd.Seekdir(ctx, input.Offset)
			if errno != 0 {
				return errnoToStatus(errno)
			}
			f.dirOffset = input.Offset
			f.overflowErrno = 0
			f.hasOverflow = false
		} else {
			return fuse.ENOTSUP
		}
	}

	defer func() {
		f.dirOffset = out.Offset
	}()

	first := true
	f.lastRead = f.lastRead[:0]
	for {
		var de *fuse.DirEntry
		var errno syscall.Errno
		if f.hasOverflow && !interruptedRead {
			f.hasOverflow = false
			if f.overflowErrno != 0 {
				return errnoToStatus(f.overflowErrno)
			}
			de = &f.overflow
		} else {
			de, errno = getdent(ctx)
			if errno != 0 {
				if first {
					return errnoToStatus(errno)
				} else {
					f.hasOverflow = true
					f.overflowErrno = errno
					return fuse.OK
				}
			}
		}

		if de == nil {
			break
		}

		first = false
		if de.Off == 0 {
			// This logic is dup from fuse.DirEntryList, but we need the offset here so it is part of lastRead
			de.Off = out.Offset + 1
		}
		if !lookup {
			if !out.AddDirEntry(*de) {
				f.overflow = *de
				f.hasOverflow = true
				return fuse.OK
			}

			f.lastRead = append(f.lastRead, *de)
			continue
		}

		entryOut := out.AddDirLookupEntry(*de)
		if entryOut == nil {
			f.overflow = *de
			f.hasOverflow = true
			return fuse.OK
		}
		f.lastRead = append(f.lastRead, *de)

		// Virtual entries "." and ".." should be part of the
		// directory listing, but not part of the filesystem tree.
		// The values in EntryOut are ignored by Linux
		// (see fuse_direntplus_link() in linux/fs/fuse/readdir.c), so leave
		// them at zero-value.
		if de.Name == "." || de.Name == ".." {
			continue
		}

		var child *Inode
		if fileLookupper, ok := f.file.(FileLookuper); ok {
			child, errno = fileLookupper.Lookup(ctx, de.Name, entryOut)
		} else {
			child, errno = b.lookup(ctx, n, de.Name, entryOut)
		}

		if errno != 0 {
			if b.options.NegativeTimeout != nil {
				entryOut.SetEntryTimeout(*b.options.NegativeTimeout)

				// TODO: maybe simply not produce the dirent here?
				// test?
			}
			// TODO: should break?
		} else {
			child, _, _ = b.addNewChild(n, de.Name, child, nil, 0, entryOut)
			child.setEntryOut(entryOut)
			b.setEntryOutTimeout(entryOut)
			if de.Mode&syscall.S_IFMT != child.stableAttr.Mode&syscall.S_IFMT {
				// The file type has changed behind our back. Use the new value.
				out.FixMode(child.stableAttr.Mode)
			}
			entryOut.Mode = child.stableAttr.Mode | (entryOut.Mode & 07777)
		}
	}

	return fuse.OK
}

func (b *rawBridge) FsyncDir(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if fsd, ok := f.file.(FileFsyncdirer); ok {
		return errnoToStatus(fsd.Fsyncdir(ctx, input.FsyncFlags))
	} else if fs, ok := n.ops.(NodeFsyncer); ok {
		return errnoToStatus(fs.Fsync(ctx, f.file, input.FsyncFlags))
	}

	return fuse.ENOTSUP
}

func (b *rawBridge) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	if sf, ok := n.ops.(NodeStatfser); ok {
		return errnoToStatus(sf.Statfs(&fuse.Context{Caller: input.Caller, Cancel: cancel}, out))
	}

	// leave zeroed out
	return fuse.OK
}

func (b *rawBridge) Init(s *fuse.Server) {
	b.server = s
}

func (b *rawBridge) CopyFileRange(cancel <-chan struct{}, in *fuse.CopyFileRangeIn) (size uint32, status fuse.Status) {
	n1, f1 := b.inode(in.NodeId, in.FhIn)
	cfr, ok := n1.ops.(NodeCopyFileRanger)
	if !ok {
		return 0, fuse.ENOTSUP
	}

	n2, f2 := b.inode(in.NodeIdOut, in.FhOut)

	sz, errno := cfr.CopyFileRange(&fuse.Context{Caller: in.Caller, Cancel: cancel},
		f1.file, in.OffIn, n2, f2.file, in.OffOut, in.Len, in.Flags)
	return sz, errnoToStatus(errno)
}

func (b *rawBridge) Ioctl(cancel <-chan struct{}, in *fuse.IoctlIn, inbuf []byte, out *fuse.IoctlOut, outbuf []byte) (code fuse.Status) {
	n, f := b.inode(in.NodeId, in.Fh)
	if nio, ok := n.ops.(NodeIoctler); ok {
		ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}
		result, errno := nio.Ioctl(ctx, f.file, in.Cmd, in.Arg, inbuf, outbuf)
		out.Result = result
		return errnoToStatus(errno)
	}
	if fio, ok := f.file.(FileIoctler); ok {
		ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}
		result, errno := fio.Ioctl(ctx, in.Cmd, in.Arg, inbuf, outbuf)
		out.Result = result
		return errnoToStatus(errno)
	}
	return fuse.Status(syscall.ENOTTY)
}

func (b *rawBridge) Lseek(cancel <-chan struct{}, in *fuse.LseekIn, out *fuse.LseekOut) fuse.Status {
	n, f := b.inode(in.NodeId, in.Fh)

	ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}

	ls, ok := n.ops.(NodeLseeker)
	if ok {
		off, errno := ls.Lseek(ctx,
			f.file, in.Offset, in.Whence)
		out.Offset = off
		return errnoToStatus(errno)
	}
	if fs, ok := f.file.(FileLseeker); ok {
		off, errno := fs.Lseek(ctx, in.Offset, in.Whence)
		out.Offset = off
		return errnoToStatus(errno)
	}
	var attr fuse.AttrOut
	if s := b.getattr(ctx, n, nil, &attr); s != 0 {
		return errnoToStatus(s)
	}
	if in.Whence == _SEEK_DATA {
		if in.Offset >= attr.Size {
			return errnoToStatus(syscall.ENXIO)
		}
		out.Offset = in.Offset
		return fuse.OK
	}

	if in.Whence == _SEEK_HOLE {
		if in.Offset > attr.Size {
			return errnoToStatus(syscall.ENXIO)
		}
		out.Offset = attr.Size
		return fuse.OK
	}

	return fuse.ENOTSUP
}

func (b *rawBridge) OnUnmount() {
	if of, ok := b.root.ops.(NodeOnForgetter); ok {
		of.OnForget()
	}
}
