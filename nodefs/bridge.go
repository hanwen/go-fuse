// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"golang.org/x/sys/unix"
)

func errnoToStatus(errno syscall.Errno) fuse.Status {
	return fuse.Status(errno)
}

type fileEntry struct {
	file FileHandle

	// index into Inode.openFiles
	nodeIndex int

	// Directory
	dirStream   DirStream
	hasOverflow bool
	overflow    fuse.DirEntry

	wg sync.WaitGroup
}

type rawBridge struct {
	options Options
	root    *Inode
	server  *fuse.Server

	// mu protects the following data.  Locks for inodes must be
	// taken before rawBridge.mu
	mu           sync.Mutex
	nodes        map[uint64]*Inode
	automaticIno uint64

	files     []*fileEntry
	freeFiles []uint32
}

// newInode creates creates new inode pointing to ops.
func (b *rawBridge) newInode(ctx context.Context, ops Operations, id NodeAttr, persistent bool) *Inode {
	b.mu.Lock()
	defer b.mu.Unlock()

	if id.Reserved() {
		log.Panicf("using reserved ID %d for inode number", id.Ino)
	}

	if id.Ino == 0 {
		id.Ino = b.automaticIno
		b.automaticIno++
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
	// simultaneously.  The matching NodeAttrs ensure that we return the
	// same node.
	old := b.nodes[id.Ino]
	if old != nil {
		return old
	}
	id.Mode = id.Mode &^ 07777
	if id.Mode == 0 {
		id.Mode = fuse.S_IFREG
	}

	switch id.Mode {
	case fuse.S_IFDIR:
		_ = ops.(DirOperations)
	case fuse.S_IFLNK:
		_ = ops.(SymlinkOperations)
	case fuse.S_IFREG:
		_ = ops.(FileOperations)
	default:
		log.Panicf("filetype %o unimplemented", id.Mode)
	}

	inode := &Inode{
		ops:        ops,
		nodeAttr:   id,
		bridge:     b,
		persistent: persistent,
		parents:    make(map[parentData]struct{}),
	}
	if id.Mode == fuse.S_IFDIR {
		inode.children = make(map[string]*Inode)
	}

	b.nodes[id.Ino] = inode
	ops.setInode(inode)
	newIno := ops.inode()

	if newIno == inode {
		newIno.ops.OnAdd(ctx)
	}

	return newIno
}

// addNewChild inserts the child into the tree. Returns file handle if file != nil.
func (b *rawBridge) addNewChild(parent *Inode, name string, child *Inode, file FileHandle, fileFlags uint32, out *fuse.EntryOut) uint32 {
	lockNodes(parent, child)
	parent.setEntry(name, child)
	b.mu.Lock()

	child.lookupCount++

	var fh uint32
	if file != nil {
		fh = b.registerFile(child, file, fileFlags)
	}

	out.NodeId = child.nodeAttr.Ino
	out.Generation = child.nodeAttr.Gen
	out.Attr.Ino = child.nodeAttr.Ino

	b.mu.Unlock()
	unlockNodes(parent, child)
	return fh
}

func (b *rawBridge) setEntryOutTimeout(out *fuse.EntryOut) {
	if b.options.AttrTimeout != nil {
		out.SetAttrTimeout(*b.options.AttrTimeout)
	}
	if b.options.EntryTimeout != nil {
		out.SetEntryTimeout(*b.options.EntryTimeout)
	}
}

func (b *rawBridge) setAttrTimeout(out *fuse.AttrOut) {
	if b.options.AttrTimeout != nil {
		out.SetTimeout(*b.options.AttrTimeout)
	}
}

// NewNodeFS creates a node based filesystem based on an Operations
// instance for the root.
func NewNodeFS(root DirOperations, opts *Options) fuse.RawFileSystem {
	bridge := &rawBridge{
		automaticIno: opts.FirstAutomaticIno,
	}
	if bridge.automaticIno == 1 {
		bridge.automaticIno++
	}

	if bridge.automaticIno == 0 {
		bridge.automaticIno = 1 << 63
	}

	if opts != nil {
		bridge.options = *opts
	} else {
		oneSec := time.Second
		bridge.options.EntryTimeout = &oneSec
		bridge.options.AttrTimeout = &oneSec
	}

	bridge.root = &Inode{
		lookupCount: 1,
		children:    make(map[string]*Inode),
		parents:     nil,
		ops:         root,
		bridge:      bridge,
		nodeAttr: NodeAttr{
			Ino:  1,
			Mode: fuse.S_IFDIR,
		},
	}
	root.setInode(bridge.root)
	bridge.nodes = map[uint64]*Inode{
		1: bridge.root,
	}

	// Fh 0 means no file handle.
	bridge.files = []*fileEntry{{}}

	root.OnAdd(context.Background())

	return bridge
}

func (b *rawBridge) String() string {
	return "rawBridge"
}

func (b *rawBridge) inode(id uint64, fh uint64) (*Inode, *fileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, f := b.nodes[id], b.files[fh]
	if n == nil {
		log.Panicf("unknown node %d", id)
	}
	return n, f
}

func (b *rawBridge) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)

	child, errno := parent.dirOps().Lookup(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name, out)
	if errno != 0 {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return errnoToStatus(errno)
	}

	b.addNewChild(parent, name, child, nil, 0, out)
	b.setEntryOutTimeout(out)

	out.Mode = child.nodeAttr.Mode | (out.Mode & 07777)
	return fuse.OK
}

func (b *rawBridge) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var errno syscall.Errno
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		errno = mops.Rmdir(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	if errno == 0 {
		parent.RmChild(name)
	}
	return errnoToStatus(errno)
}

func (b *rawBridge) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var errno syscall.Errno
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		errno = mops.Unlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	if errno == 0 {
		parent.RmChild(name)
	}
	return errnoToStatus(errno)
}

func (b *rawBridge) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	var child *Inode
	var errno syscall.Errno
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, errno = mops.Mkdir(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, out)
	}

	if errno != 0 {
		return errnoToStatus(errno)
	}

	if out.Attr.Mode&^07777 != fuse.S_IFDIR {
		log.Panicf("Mkdir: mode must be S_IFDIR (%o), got %o", fuse.S_IFDIR, out.Attr.Mode)
	}

	b.addNewChild(parent, name, child, nil, 0, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	var child *Inode
	var errno syscall.Errno
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, errno = mops.Mknod(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, input.Rdev, out)
	}

	if errno != 0 {
		return errnoToStatus(errno)
	}

	b.addNewChild(parent, name, child, nil, 0, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) fuse.Status {
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	parent, _ := b.inode(input.NodeId, 0)

	var child *Inode
	var errno syscall.Errno
	var f FileHandle
	var flags uint32
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, f, flags, errno = mops.Create(ctx, name, input.Flags, input.Mode)
	}

	if errno != 0 {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return errnoToStatus(errno)
	}

	out.Fh = uint64(b.addNewChild(parent, name, child, f, input.Flags|syscall.O_CREAT, &out.EntryOut))
	b.setEntryOutTimeout(&out.EntryOut)

	out.OpenFlags = flags

	var temp fuse.AttrOut
	f.GetAttr(ctx, &temp)
	out.Attr = temp.Attr
	out.AttrValid = temp.AttrValid
	out.AttrValidNsec = temp.AttrValidNsec
	out.Attr.Ino = child.nodeAttr.Ino
	out.Generation = child.nodeAttr.Gen
	out.NodeId = child.nodeAttr.Ino

	b.setEntryOutTimeout(&out.EntryOut)
	out.Mode = (out.Attr.Mode & 07777) | child.nodeAttr.Mode
	return fuse.OK
}

func (b *rawBridge) Forget(nodeid, nlookup uint64) {
	n, _ := b.inode(nodeid, 0)
	n.removeRef(nlookup, false)
}

func (b *rawBridge) SetDebug(debug bool) {}

func (b *rawBridge) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	n, fEntry := b.inode(input.NodeId, input.Fh())
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	if fops, ok := n.ops.(FileOperations); ok {

		f := fEntry.file
		if input.Flags()&fuse.FUSE_GETATTR_FH == 0 {
			// The linux kernel doesnt pass along the file
			// descriptor, so we have to fake it here.
			// See https://github.com/libfuse/libfuse/issues/62
			b.mu.Lock()
			for _, fh := range n.openFiles {
				f = b.files[fh].file
				b.files[fh].wg.Add(1)
				defer b.files[fh].wg.Done()
				break
			}
			b.mu.Unlock()
		}

		errno := fops.FGetAttr(ctx, f, out)
		b.setAttrTimeout(out)
		out.Ino = input.NodeId
		out.Mode = (out.Attr.Mode & 07777) | n.nodeAttr.Mode
		return errnoToStatus(errno)
	}
	return errnoToStatus(n.ops.GetAttr(ctx, out))
}

func (b *rawBridge) SetAttr(cancel <-chan struct{}, in *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}

	n, fEntry := b.inode(in.NodeId, in.Fh)
	f := fEntry.file
	if in.Valid&fuse.FATTR_FH == 0 {
		f = nil
	}

	if fops, ok := n.ops.(FileOperations); ok {
		return errnoToStatus(fops.FSetAttr(ctx, f, in, out))
	}

	return errnoToStatus(n.ops.SetAttr(ctx, in, out))
}

func (b *rawBridge) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) fuse.Status {
	p1, _ := b.inode(input.NodeId, 0)
	p2, _ := b.inode(input.Newdir, 0)

	if mops, ok := p1.ops.(MutableDirOperations); ok {
		errno := mops.Rename(&fuse.Context{Caller: input.Caller, Cancel: cancel}, oldName, p2.ops, newName, input.Flags)
		if errno == 0 {
			if input.Flags&unix.RENAME_EXCHANGE != 0 {
				p1.ExchangeChild(oldName, p2, newName)
			} else {
				p1.MvChild(oldName, p2, newName, true)
			}

			return errnoToStatus(errno)
		}
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Link(cancel <-chan struct{}, input *fuse.LinkIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)
	target, _ := b.inode(input.Oldnodeid, 0)

	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, errno := mops.Link(&fuse.Context{Caller: input.Caller, Cancel: cancel}, target.ops, name, out)
		if errno != 0 {
			return errnoToStatus(errno)
		}

		b.addNewChild(parent, name, child, nil, 0, out)
		b.setEntryOutTimeout(out)
		return fuse.OK
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Symlink(cancel <-chan struct{}, header *fuse.InHeader, target string, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)

	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, status := mops.Symlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, target, name, out)
		if status != 0 {
			return errnoToStatus(status)
		}

		b.addNewChild(parent, name, child, nil, 0, out)
		b.setEntryOutTimeout(out)
		return fuse.OK
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	result, errno := n.linkOps().Readlink(&fuse.Context{Caller: header.Caller, Cancel: cancel})
	if errno != 0 {
		return nil, errnoToStatus(errno)
	}

	return result, fuse.OK
}

func (b *rawBridge) Access(cancel <-chan struct{}, input *fuse.AccessIn) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	return errnoToStatus(n.ops.Access(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Mask))
}

// Extended attributes.

func (b *rawBridge) GetXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string, data []byte) (uint32, fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)

	if xops, ok := n.ops.(XAttrOperations); ok {
		nb, errno := xops.GetXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr, data)
		return nb, errnoToStatus(errno)
	}

	return 0, fuse.ENOSYS
}

func (b *rawBridge) ListXAttr(cancel <-chan struct{}, header *fuse.InHeader, dest []byte) (sz uint32, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		sz, errno := xops.ListXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, dest)
		return sz, errnoToStatus(errno)
	}
	return 0, fuse.ENOSYS
}

func (b *rawBridge) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		return errnoToStatus(xops.SetXAttr(&fuse.Context{Caller: input.Caller, Cancel: cancel}, attr, data, input.Flags))
	}
	return fuse.ENOSYS
}

func (b *rawBridge) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) fuse.Status {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		return errnoToStatus(xops.RemoveXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr))
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	f, flags, errno := n.fileOps().Open(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Flags)
	if errno != 0 {
		return errnoToStatus(errno)
	}

	if f != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		out.Fh = uint64(b.registerFile(n, f, input.Flags))
	}
	out.OpenFlags = flags
	return fuse.OK
}

// registerFile hands out a file handle. Must have bridge.mu
func (b *rawBridge) registerFile(n *Inode, f FileHandle, flags uint32) uint32 {
	var fh uint32
	if len(b.freeFiles) > 0 {
		last := len(b.freeFiles) - 1
		fh = b.freeFiles[last]
		b.freeFiles = b.freeFiles[:last]
	} else {
		fh = uint32(len(b.files))
		b.files = append(b.files, &fileEntry{})
	}

	fileEntry := b.files[fh]
	fileEntry.nodeIndex = len(n.openFiles)
	fileEntry.file = f

	n.openFiles = append(n.openFiles, fh)
	return fh
}

func (b *rawBridge) Read(cancel <-chan struct{}, input *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	res, errno := n.fileOps().Read(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, buf, int64(input.Offset))
	return res, errnoToStatus(errno)
}

func (b *rawBridge) GetLk(cancel <-chan struct{}, input *fuse.LkIn, out *fuse.LkOut) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	if lops, ok := n.ops.(LockOperations); ok {
		return errnoToStatus(lops.GetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags, &out.Lk))
	}
	return fuse.ENOSYS
}

func (b *rawBridge) SetLk(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	if lops, ok := n.ops.(LockOperations); ok {
		return errnoToStatus(lops.SetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags))
	}
	return fuse.ENOSYS
}
func (b *rawBridge) SetLkw(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	if lops, ok := n.ops.(LockOperations); ok {
		return errnoToStatus(lops.SetLkw(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags))
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Release(input *fuse.ReleaseIn) {
	// XXX should have cancel channel too.
	n, f := b.releaseFileEntry(input.NodeId, input.Fh)
	if f == nil {
		return
	}

	f.wg.Wait()
	n.fileOps().Release(&fuse.Context{Caller: input.Caller, Cancel: nil}, f.file)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.freeFiles = append(b.freeFiles, uint32(input.Fh))
}

func (b *rawBridge) ReleaseDir(input *fuse.ReleaseIn) {
	_, f := b.releaseFileEntry(input.NodeId, input.Fh)
	f.wg.Wait()
	if f.dirStream != nil {
		f.dirStream.Close()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.freeFiles = append(b.freeFiles, uint32(input.Fh))
}

func (b *rawBridge) releaseFileEntry(nid uint64, fh uint64) (*Inode, *fileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := b.nodes[nid]
	var entry *fileEntry
	if fh > 0 {
		last := len(n.openFiles) - 1
		entry = b.files[fh]
		if last != entry.nodeIndex {
			n.openFiles[entry.nodeIndex] = n.openFiles[last]

			b.files[n.openFiles[entry.nodeIndex]].nodeIndex = entry.nodeIndex
		}
		n.openFiles = n.openFiles[:last]
	}
	return n, entry
}

func (b *rawBridge) Write(cancel <-chan struct{}, input *fuse.WriteIn, data []byte) (written uint32, status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)

	w, errno := n.fileOps().Write(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, data, int64(input.Offset))
	return w, errnoToStatus(errno)
}

func (b *rawBridge) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return errnoToStatus(n.fileOps().Flush(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file))
}

func (b *rawBridge) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return errnoToStatus(n.fileOps().Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.FsyncFlags))
}

func (b *rawBridge) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return errnoToStatus(n.fileOps().Allocate(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Offset, input.Length, input.Mode))
}

func (b *rawBridge) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	errno := n.dirOps().OpenDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
	if errno != 0 {
		return errnoToStatus(errno)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out.Fh = uint64(b.registerFile(n, nil, 0))
	return fuse.OK
}

func (b *rawBridge) getStream(cancel <-chan struct{}, input *fuse.ReadIn, inode *Inode, f *fileEntry) syscall.Errno {
	if f.dirStream == nil || input.Offset == 0 {
		if f.dirStream != nil {
			f.dirStream.Close()
			f.dirStream = nil
		}
		str, errno := inode.dirOps().ReadDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
		if errno != 0 {
			return errno
		}

		f.hasOverflow = false
		f.dirStream = str
	}

	return 0
}

func (b *rawBridge) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	if errno := b.getStream(cancel, input, n, f); errno != 0 {
		return errnoToStatus(errno)
	}

	if f.hasOverflow {
		// always succeeds.
		out.AddDirEntry(f.overflow)
		f.hasOverflow = false
	}

	for f.dirStream.HasNext() {
		e, errno := f.dirStream.Next()

		if errno != 0 {
			return errnoToStatus(errno)
		}
		if !out.AddDirEntry(e) {
			f.overflow = e
			f.hasOverflow = true
			return errnoToStatus(errno)
		}
	}

	return fuse.OK
}

func (b *rawBridge) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	if errno := b.getStream(cancel, input, n, f); errno != 0 {
		return errnoToStatus(errno)
	}

	for f.dirStream.HasNext() {
		var e fuse.DirEntry
		var errno syscall.Errno
		if f.hasOverflow {
			e = f.overflow
			f.hasOverflow = false
		} else {
			e, errno = f.dirStream.Next()
		}

		if errno != 0 {
			return errnoToStatus(errno)
		}

		entryOut := out.AddDirLookupEntry(e)
		if entryOut == nil {
			f.overflow = e
			f.hasOverflow = true
			return fuse.OK
		}

		child, errno := n.dirOps().Lookup(&fuse.Context{Caller: input.Caller, Cancel: cancel}, e.Name, entryOut)
		if errno != 0 {
			if b.options.NegativeTimeout != nil {
				entryOut.SetEntryTimeout(*b.options.NegativeTimeout)
			}
		} else {
			b.addNewChild(n, e.Name, child, nil, 0, entryOut)
			b.setEntryOutTimeout(entryOut)
			if (e.Mode &^ 07777) != (child.nodeAttr.Mode &^ 07777) {
				// should go back and change the
				// already serialized entry
				log.Panicf("mode mismatch between readdir %o and lookup %o", e.Mode, child.nodeAttr.Mode)
			}
			entryOut.Mode = child.nodeAttr.Mode | (entryOut.Mode & 07777)
		}
	}

	return fuse.OK
}

func (b *rawBridge) FsyncDir(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	n, _ := b.inode(input.NodeId, input.Fh)
	return errnoToStatus(n.fileOps().Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, nil, input.FsyncFlags))
}

func (b *rawBridge) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	return errnoToStatus(n.ops.StatFs(&fuse.Context{Caller: input.Caller, Cancel: cancel}, out))
}

func (b *rawBridge) Init(s *fuse.Server) {
	b.server = s
}

func (b *rawBridge) CopyFileRange(cancel <-chan struct{}, in *fuse.CopyFileRangeIn) (size uint32, status fuse.Status) {
	n1, f1 := b.inode(in.NodeId, in.FhIn)
	n2, f2 := b.inode(in.NodeIdOut, in.FhOut)

	sz, errno := n1.fileOps().CopyFileRange(&fuse.Context{Caller: in.Caller, Cancel: cancel},
		f1.file, in.OffIn, n2, f2.file, in.OffOut, in.Len, in.Flags)
	return sz, errnoToStatus(errno)
}

func (b *rawBridge) Lseek(cancel <-chan struct{}, in *fuse.LseekIn, out *fuse.LseekOut) fuse.Status {
	n, f := b.inode(in.NodeId, in.Fh)

	off, errno := n.fileOps().Lseek(&fuse.Context{Caller: in.Caller, Cancel: cancel},
		f.file, in.Offset, in.Whence)
	out.Offset = off
	return errnoToStatus(errno)
}
