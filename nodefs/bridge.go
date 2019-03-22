// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"golang.org/x/sys/unix"
)

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
func (b *rawBridge) newInode(ops Operations, id NodeAttr, persistent bool) *Inode {
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
		nodeID:     id,
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
		newIno.ops.OnAdd()
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

	out.NodeId = child.nodeID.Ino
	out.Generation = child.nodeID.Gen
	out.Attr.Ino = child.nodeID.Ino

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
		nodeID: NodeAttr{
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

	root.OnAdd()

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

func (b *rawBridge) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(header.NodeId, 0)

	child, status := parent.dirOps().Lookup(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name, out)
	if !status.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return status
	}

	b.addNewChild(parent, name, child, nil, 0, out)
	b.setEntryOutTimeout(out)

	out.Mode = child.nodeID.Mode | (out.Mode & 07777)
	return fuse.OK
}

func (b *rawBridge) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var status fuse.Status
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		status = mops.Rmdir(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	if status.Ok() {
		parent.RmChild(name)
	}
	return status

}

func (b *rawBridge) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	var status fuse.Status
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		status = mops.Unlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	}

	if status.Ok() {
		parent.RmChild(name)
	}
	return status
}

func (b *rawBridge) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) fuse.Status {
	parent, _ := b.inode(input.NodeId, 0)

	var child *Inode
	var status fuse.Status
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, status = mops.Mkdir(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, out)
	}

	if !status.Ok() {
		return status
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
	var status fuse.Status
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, status = mops.Mknod(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, input.Rdev, out)
	}

	if !status.Ok() {
		return status
	}

	b.addNewChild(parent, name, child, nil, 0, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) fuse.Status {
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	parent, _ := b.inode(input.NodeId, 0)

	var child *Inode
	var status fuse.Status
	var f FileHandle
	var flags uint32
	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, f, flags, status = mops.Create(ctx, name, input.Flags, input.Mode)
	}

	if !status.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return status
	}

	out.Fh = uint64(b.addNewChild(parent, name, child, f, input.Flags|syscall.O_CREAT, &out.EntryOut))
	b.setEntryOutTimeout(&out.EntryOut)

	out.OpenFlags = flags

	var temp fuse.AttrOut
	f.GetAttr(ctx, &temp)
	out.Attr = temp.Attr
	out.AttrValid = temp.AttrValid
	out.AttrValidNsec = temp.AttrValidNsec
	out.Attr.Ino = child.nodeID.Ino
	out.Generation = child.nodeID.Gen
	out.NodeId = child.nodeID.Ino

	b.setEntryOutTimeout(&out.EntryOut)
	out.Mode = (out.Attr.Mode & 07777) | child.nodeID.Mode
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

		status := fops.FGetAttr(ctx, f, out)
		b.setAttrTimeout(out)
		out.Ino = input.NodeId
		out.Mode = (out.Attr.Mode & 07777) | n.nodeID.Mode
		return status
	}
	return n.ops.GetAttr(ctx, out)
}

func (b *rawBridge) SetAttr(cancel <-chan struct{}, in *fuse.SetAttrIn, out *fuse.AttrOut) (status fuse.Status) {
	ctx := &fuse.Context{Caller: in.Caller, Cancel: cancel}

	n, fEntry := b.inode(in.NodeId, in.Fh)
	f := fEntry.file
	if in.Valid&fuse.FATTR_FH == 0 {
		f = nil
	}

	if fops, ok := n.ops.(FileOperations); ok {
		return fops.FSetAttr(ctx, f, in, out)
	}

	return n.ops.SetAttr(ctx, in, out)
}

func (b *rawBridge) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) fuse.Status {
	p1, _ := b.inode(input.NodeId, 0)
	p2, _ := b.inode(input.Newdir, 0)

	if mops, ok := p1.ops.(MutableDirOperations); ok {
		status := mops.Rename(&fuse.Context{Caller: input.Caller, Cancel: cancel}, oldName, p2.ops, newName, input.Flags)
		if status.Ok() {
			if input.Flags&unix.RENAME_EXCHANGE != 0 {
				p1.ExchangeChild(oldName, p2, newName)
			} else {
				p1.MvChild(oldName, p2, newName, true)
			}

			return status
		}
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Link(cancel <-chan struct{}, input *fuse.LinkIn, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)
	target, _ := b.inode(input.Oldnodeid, 0)

	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, status := mops.Link(&fuse.Context{Caller: input.Caller, Cancel: cancel}, target.ops, name, out)
		if !status.Ok() {
			return status
		}

		b.addNewChild(parent, name, child, nil, 0, out)
		b.setEntryOutTimeout(out)
		return fuse.OK
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Symlink(cancel <-chan struct{}, header *fuse.InHeader, target string, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(header.NodeId, 0)

	if mops, ok := parent.ops.(MutableDirOperations); ok {
		child, status := mops.Symlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, target, name, out)
		if !status.Ok() {
			return status
		}

		b.addNewChild(parent, name, child, nil, 0, out)
		b.setEntryOutTimeout(out)
		return fuse.OK
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	result, status := n.linkOps().Readlink(&fuse.Context{Caller: header.Caller, Cancel: cancel})
	if !status.Ok() {
		return nil, status
	}

	return []byte(result), fuse.OK
}

func (b *rawBridge) Access(cancel <-chan struct{}, input *fuse.AccessIn) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	return n.ops.Access(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Mask)
}

// Extended attributes.

func (b *rawBridge) GetXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string, data []byte) (uint32, fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)

	if xops, ok := n.ops.(XAttrOperations); ok {
		return xops.GetXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr, data)
	}

	return 0, fuse.ENOSYS
}

func (b *rawBridge) ListXAttr(cancel <-chan struct{}, header *fuse.InHeader, dest []byte) (sz uint32, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		return xops.ListXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, dest)
	}
	return 0, fuse.ENOSYS
}

func (b *rawBridge) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		return xops.SetXAttr(&fuse.Context{Caller: input.Caller, Cancel: cancel}, attr, data, input.Flags)
	}
	return fuse.ENOSYS
}

func (b *rawBridge) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) (status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	if xops, ok := n.ops.(XAttrOperations); ok {
		return xops.RemoveXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr)
	}
	return fuse.ENOSYS
}

func (b *rawBridge) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	// NOSUBMIT: what about the mode argument?
	f, flags, status := n.fileOps().Open(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Flags)
	if !status.Ok() {
		return status
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
	return n.fileOps().Read(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, buf, int64(input.Offset))
}

func (b *rawBridge) GetLk(cancel <-chan struct{}, input *fuse.LkIn, out *fuse.LkOut) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)

	if lops, ok := n.ops.(LockOperations); ok {
		return lops.GetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags, &out.Lk)
	}
	return fuse.ENOSYS
}

func (b *rawBridge) SetLk(cancel <-chan struct{}, input *fuse.LkIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	if lops, ok := n.ops.(LockOperations); ok {
		return lops.SetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags)
	}
	return fuse.ENOSYS
}
func (b *rawBridge) SetLkw(cancel <-chan struct{}, input *fuse.LkIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	if lops, ok := n.ops.(LockOperations); ok {
		return lops.SetLkw(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags)
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
	return n.fileOps().Write(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, data, int64(input.Offset))
}

func (b *rawBridge) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.fileOps().Flush(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file)
}

func (b *rawBridge) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.fileOps().Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.FsyncFlags)
}

func (b *rawBridge) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.fileOps().Allocate(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Offset, input.Length, input.Mode)
}

func (b *rawBridge) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	status := n.dirOps().OpenDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
	if !status.Ok() {
		return status
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out.Fh = uint64(b.registerFile(n, nil, 0))
	return fuse.OK
}

func (b *rawBridge) getStream(cancel <-chan struct{}, input *fuse.ReadIn, inode *Inode, f *fileEntry) fuse.Status {
	if f.dirStream == nil || input.Offset == 0 {
		if f.dirStream != nil {
			f.dirStream.Close()
			f.dirStream = nil
		}
		str, status := inode.dirOps().ReadDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
		if !status.Ok() {
			return status
		}

		f.hasOverflow = false
		f.dirStream = str
	}

	return fuse.OK
}

func (b *rawBridge) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	if status := b.getStream(cancel, input, n, f); !status.Ok() {
		return status
	}

	if f.hasOverflow {
		// always succeeds.
		out.AddDirEntry(f.overflow)
		f.hasOverflow = false
	}

	for f.dirStream.HasNext() {
		e, status := f.dirStream.Next()

		if !status.Ok() {
			return status
		}
		if !out.AddDirEntry(e) {
			f.overflow = e
			f.hasOverflow = true
			return status
		}
	}

	return fuse.OK
}

func (b *rawBridge) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)

	if status := b.getStream(cancel, input, n, f); !status.Ok() {
		return status
	}

	for f.dirStream.HasNext() {
		var e fuse.DirEntry
		var status fuse.Status
		if f.hasOverflow {
			e = f.overflow
			f.hasOverflow = false
		} else {
			e, status = f.dirStream.Next()
		}

		if !status.Ok() {
			return status
		}

		entryOut := out.AddDirLookupEntry(e)
		if entryOut == nil {
			f.overflow = e
			f.hasOverflow = true
			return fuse.OK
		}

		child, status := n.dirOps().Lookup(&fuse.Context{Caller: input.Caller, Cancel: cancel}, e.Name, entryOut)
		if !status.Ok() {
			if b.options.NegativeTimeout != nil {
				entryOut.SetEntryTimeout(*b.options.NegativeTimeout)
			}
		} else {
			b.addNewChild(n, e.Name, child, nil, 0, entryOut)
			b.setEntryOutTimeout(entryOut)
			if (e.Mode &^ 07777) != (child.nodeID.Mode &^ 07777) {
				// should go back and change the
				// already serialized entry
				log.Panicf("mode mismatch between readdir %o and lookup %o", e.Mode, child.nodeID.Mode)
			}
			entryOut.Mode = child.nodeID.Mode | (entryOut.Mode & 07777)
		}
	}

	return fuse.OK
}

func (b *rawBridge) FsyncDir(cancel <-chan struct{}, input *fuse.FsyncIn) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, input.Fh)
	return n.fileOps().Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, nil, input.FsyncFlags)
}

func (b *rawBridge) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	return n.ops.StatFs(&fuse.Context{Caller: input.Caller, Cancel: cancel}, out)
}

func (b *rawBridge) Init(s *fuse.Server) {
	b.server = s
}
