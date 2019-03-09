// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"golang.org/x/sys/unix"
)

type fileEntry struct {
	file FileHandle

	// Directory
	dirStream   DirStream
	hasOverflow bool
	overflow    fuse.DirEntry
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
	freeFiles []uint64
}

// newInode creates creates new inode pointing to node.
func (b *rawBridge) newInode(node Operations, mode uint32, id FileID, persistent bool) *Inode {
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
	// simultaneously.  The matching FileIDs ensure that we return the
	// same node.
	old := b.nodes[id.Ino]
	if old != nil {
		return old
	}
	mode = mode &^ 07777
	inode := &Inode{
		mode:       mode,
		node:       node,
		nodeID:     id,
		bridge:     b,
		persistent: persistent,
		parents:    make(map[parentData]struct{}),
	}
	if mode == fuse.S_IFDIR {
		inode.children = make(map[string]*Inode)
	}

	b.nodes[id.Ino] = inode
	node.setInode(inode)
	return node.inode()
}

func NewNodeFS(root Operations, opts *Options) fuse.RawFileSystem {
	bridge := &rawBridge{
		automaticIno: 1 << 63,
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
		mode:        fuse.S_IFDIR,
		children:    make(map[string]*Inode),
		parents:     nil,
		node:        root,
		bridge:      bridge,
	}
	bridge.root.nodeID.Ino = 1
	root.setInode(bridge.root)
	bridge.nodes = map[uint64]*Inode{
		1: bridge.root,
	}

	// Fh 0 means no file handle.
	bridge.files = []*fileEntry{{}}
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

	child, status := parent.node.Lookup(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name, out)
	if !status.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return status
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)

	out.Mode = child.mode | (out.Mode & 07777)
	return fuse.OK
}

func (b *rawBridge) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	status := parent.node.Rmdir(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	if status.Ok() {
		parent.RmChild(name)
	}
	return status

}

func (b *rawBridge) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	status := parent.node.Unlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, name)
	if status.Ok() {
		parent.RmChild(name)
	}
	return status
}

func (b *rawBridge) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)

	child, status := parent.node.Mkdir(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, out)
	if !status.Ok() {
		return status
	}

	if out.Attr.Mode&^07777 != fuse.S_IFDIR {
		log.Panicf("Mkdir: mode must be S_IFDIR (%o), got %o", fuse.S_IFDIR, out.Attr.Mode)
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)

	child, status := parent.node.Mknod(&fuse.Context{Caller: input.Caller, Cancel: cancel}, name, input.Mode, input.Rdev, out)
	if !status.Ok() {
		return status
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

// addNewChild inserts the child into the tree. Returns file handle if file != nil.
func (b *rawBridge) addNewChild(parent *Inode, name string, child *Inode, file FileHandle, out *fuse.EntryOut) uint64 {
	lockNodes(parent, child)
	parent.setEntry(name, child)
	b.mu.Lock()

	child.lookupCount++

	var fh uint64
	if file != nil {
		fh = b.registerFile(file)
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

func (b *rawBridge) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) (status fuse.Status) {
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}
	parent, _ := b.inode(input.NodeId, 0)
	child, f, flags, status := parent.node.Create(ctx, name, input.Flags, input.Mode)
	if !status.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return status
	}

	out.Fh = b.addNewChild(parent, name, child, f, &out.EntryOut)
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
	out.Mode = (out.Attr.Mode & 07777) | child.mode
	return fuse.OK
}

func (b *rawBridge) Forget(nodeid, nlookup uint64) {
	n, _ := b.inode(nodeid, 0)
	n.removeRef(nlookup, false)
}

func (b *rawBridge) SetDebug(debug bool) {}

func (b *rawBridge) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	n, fEntry := b.inode(input.NodeId, input.Fh())
	f := fEntry.file

	if input.Flags()&fuse.FUSE_GETATTR_FH == 0 {
		f = nil
	}

	status := n.node.GetAttr(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f, out)
	b.setAttrTimeout(out)
	out.Ino = input.NodeId
	out.Mode = (out.Attr.Mode & 07777) | n.mode
	return status
}

func (b *rawBridge) setAttrTimeout(out *fuse.AttrOut) {
	if b.options.AttrTimeout != nil {
		out.SetTimeout(*b.options.AttrTimeout)
	}
}

func (b *rawBridge) SetAttr(cancel <-chan struct{}, input *fuse.SetAttrIn, out *fuse.AttrOut) (status fuse.Status) {
	ctx := &fuse.Context{Caller: input.Caller, Cancel: cancel}

	n, fEntry := b.inode(input.NodeId, input.Fh)
	f := fEntry.file
	if input.Valid&fuse.FATTR_FH == 0 {
		f = nil
	}

	if input.Valid&fuse.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		status = n.node.Chmod(ctx, f, permissions)
	}

	if status.Ok() && (input.Valid&(fuse.FATTR_UID|fuse.FATTR_GID) != 0) {
		var uid uint32 = ^uint32(0) // means "do not change" in chown(2)
		var gid uint32 = ^uint32(0)
		if input.Valid&fuse.FATTR_UID != 0 {
			uid = input.Uid
		}
		if input.Valid&fuse.FATTR_GID != 0 {
			gid = input.Gid
		}
		status = n.node.Chown(ctx, f, uid, gid)
	}

	if status.Ok() && input.Valid&fuse.FATTR_SIZE != 0 {
		status = n.node.Truncate(ctx, f, input.Size)
	}

	if status.Ok() && (input.Valid&(fuse.FATTR_ATIME|fuse.FATTR_MTIME|fuse.FATTR_ATIME_NOW|fuse.FATTR_MTIME_NOW) != 0) {
		now := time.Now()
		var atime *time.Time
		var mtime *time.Time

		if input.Valid&fuse.FATTR_ATIME != 0 {
			if input.Valid&fuse.FATTR_ATIME_NOW != 0 {
				atime = &now
			} else {
				t := time.Unix(int64(input.Atime), int64(input.Atimensec))
				atime = &t
			}
		}

		if input.Valid&fuse.FATTR_MTIME != 0 {
			if input.Valid&fuse.FATTR_MTIME_NOW != 0 {
				mtime = &now
			} else {
				t := time.Unix(int64(input.Mtime), int64(input.Mtimensec))
				mtime = &t
			}
		}

		status = n.node.Utimens(ctx, f, atime, mtime)
	}

	if !status.Ok() {
		return status
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	status = n.node.GetAttr(ctx, f, out)
	b.setAttrTimeout(out)
	out.Ino = n.nodeID.Ino
	out.Mode = n.mode | (out.Mode &^ 07777)
	return status
}

func (b *rawBridge) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) fuse.Status {
	p1, _ := b.inode(input.NodeId, 0)
	p2, _ := b.inode(input.Newdir, 0)

	status := p1.node.Rename(&fuse.Context{Caller: input.Caller, Cancel: cancel}, oldName, p2.node, newName, input.Flags)
	if status.Ok() {
		if input.Flags&unix.RENAME_EXCHANGE != 0 {
			// XXX - test coverage.
			p1.ExchangeChild(oldName, p2, newName)
		} else {
			p1.MvChild(oldName, p2, newName, true)
		}
	}
	return status
}

func (b *rawBridge) Link(cancel <-chan struct{}, input *fuse.LinkIn, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)
	target, _ := b.inode(input.Oldnodeid, 0)

	child, status := parent.node.Link(&fuse.Context{Caller: input.Caller, Cancel: cancel}, target.node, name, out)
	if !status.Ok() {
		return status
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Symlink(cancel <-chan struct{}, header *fuse.InHeader, target string, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(header.NodeId, 0)
	child, status := parent.node.Symlink(&fuse.Context{Caller: header.Caller, Cancel: cancel}, target, name, out)
	if !status.Ok() {
		return status
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Readlink(cancel <-chan struct{}, header *fuse.InHeader) (out []byte, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	result, status := n.node.Readlink(&fuse.Context{Caller: header.Caller, Cancel: cancel})
	if !status.Ok() {
		return nil, status
	}

	return []byte(result), fuse.OK
}

func (b *rawBridge) Access(cancel <-chan struct{}, input *fuse.AccessIn) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	return n.node.Access(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Mask)
}

// Extended attributes.

func (b *rawBridge) GetXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string, data []byte) (uint32, fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)

	return n.node.GetXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr, data)
}

func (b *rawBridge) ListXAttr(cancel <-chan struct{}, header *fuse.InHeader, dest []byte) (sz uint32, status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	return n.node.ListXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, dest)
}

func (b *rawBridge) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	return n.node.SetXAttr(&fuse.Context{Caller: input.Caller, Cancel: cancel}, attr, data, input.Flags)
}

func (b *rawBridge) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) (status fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	return n.node.RemoveXAttr(&fuse.Context{Caller: header.Caller, Cancel: cancel}, attr)
}

func (b *rawBridge) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	// NOSUBMIT: what about the mode argument?
	f, flags, status := n.node.Open(&fuse.Context{Caller: input.Caller, Cancel: cancel}, input.Flags)
	if !status.Ok() {
		return status
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	out.Fh = b.registerFile(f)
	out.OpenFlags = flags
	return fuse.OK
}

// registerFile hands out a file handle. Must have bridge.mu
func (b *rawBridge) registerFile(f FileHandle) uint64 {
	var fh uint64
	if len(b.freeFiles) > 0 {
		last := uint64(len(b.freeFiles) - 1)
		fh = b.freeFiles[last]
		b.freeFiles = b.freeFiles[:last]
	} else {
		fh = uint64(len(b.files))
		b.files = append(b.files, &fileEntry{})
	}

	b.files[fh].file = f
	return fh
}

func (b *rawBridge) Read(cancel <-chan struct{}, input *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Read(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, buf, int64(input.Offset))
}

func (b *rawBridge) GetLk(cancel <-chan struct{}, input *fuse.LkIn, out *fuse.LkOut) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.GetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags, &out.Lk)
}

func (b *rawBridge) SetLk(cancel <-chan struct{}, input *fuse.LkIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.SetLk(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) SetLkw(cancel <-chan struct{}, input *fuse.LkIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.SetLkw(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) Release(input *fuse.ReleaseIn) {
	n, f := b.inode(input.NodeId, input.Fh)
	n.node.Release(f.file)

	b.releaseFileEntry(input.Fh)
}

func (b *rawBridge) ReleaseDir(input *fuse.ReleaseIn) {
	_, f := b.inode(input.NodeId, input.Fh)

	if f.dirStream != nil {
		f.dirStream.Close()
	}
	b.releaseFileEntry(input.Fh)
}

func (b *rawBridge) releaseFileEntry(fh uint64) {
	if fh > 0 {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.files[fh].file = nil
		b.freeFiles = append(b.freeFiles, fh)
	}
}

func (b *rawBridge) Write(cancel <-chan struct{}, input *fuse.WriteIn, data []byte) (written uint32, status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Write(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, data, int64(input.Offset))
}

func (b *rawBridge) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Flush(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file)
}

func (b *rawBridge) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.FsyncFlags)
}

func (b *rawBridge) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) (status fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Allocate(&fuse.Context{Caller: input.Caller, Cancel: cancel}, f.file, input.Offset, input.Length, input.Mode)
}

func (b *rawBridge) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	n, _ := b.inode(input.NodeId, 0)
	status := n.node.OpenDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
	if !status.Ok() {
		return status
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out.Fh = b.registerFile(nil)
	return fuse.OK
}

func (b *rawBridge) getStream(cancel <-chan struct{}, input *fuse.ReadIn, inode *Inode, f *fileEntry) fuse.Status {
	if f.dirStream == nil || input.Offset == 0 {
		if f.dirStream != nil {
			f.dirStream.Close()
			f.dirStream = nil
		}
		str, status := inode.node.ReadDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
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

	// TODO - should post '..' and '.' ?
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

		child, status := n.node.Lookup(&fuse.Context{Caller: input.Caller, Cancel: cancel}, e.Name, entryOut)
		if !status.Ok() {
			if b.options.NegativeTimeout != nil {
				entryOut.SetEntryTimeout(*b.options.NegativeTimeout)
			}
		} else {
			b.addNewChild(n, e.Name, child, nil, entryOut)
			b.setEntryOutTimeout(entryOut)
			if (e.Mode &^ 07777) != (child.mode &^ 07777) {
				// XXX should go back and change the
				// already serialized entry
				log.Panicf("mode mismatch between readdir %o and lookup %o", e.Mode, child.mode)
			}
			entryOut.Mode = child.mode | (entryOut.Mode & 07777)
		}
	}

	return fuse.OK
}

func (b *rawBridge) FsyncDir(cancel <-chan struct{}, input *fuse.FsyncIn) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, input.Fh)
	return n.node.Fsync(&fuse.Context{Caller: input.Caller, Cancel: cancel}, nil, input.FsyncFlags)
}

func (b *rawBridge) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	return n.node.StatFs(&fuse.Context{Caller: input.Caller, Cancel: cancel}, out)
}

func (b *rawBridge) Init(s *fuse.Server) {
	b.server = s
}
