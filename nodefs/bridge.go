// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

type mapEntry struct {
	generation uint64
	inode      *Inode
}

type fileEntry struct {
	file File

	// space to hold directory stuff
}

type rawBridge struct {
	fuse.RawFileSystem

	options Options
	root    *Inode

	// mu protects the following data.  Locks for inodes must be
	// taken before rawBridge.mu
	mu    sync.Mutex
	nodes []mapEntry
	free  []uint64

	files     []fileEntry
	freeFiles []uint64
}

func NewNodeFS(root Node, opts *Options) fuse.RawFileSystem {
	bridge := &rawBridge{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
	}

	if opts != nil {
		bridge.options = *opts
	} else {
		oneSec := time.Second
		bridge.options.EntryTimeout = &oneSec
		bridge.options.AttrTimeout = &oneSec
	}

	bridge.root = &Inode{
		nodeID:      1,
		lookupCount: 1,
		mode:        fuse.S_IFDIR,
		children:    make(map[string]*Inode),
		parents:     nil,
		node:        root,
		bridge:      bridge,
	}
	root.setInode(bridge.root)
	bridge.nodes = append(bridge.nodes,
		mapEntry{},
		// ID 1 is always the root.
		mapEntry{inode: bridge.root})

	// Fh 0 means no file handle.
	bridge.files = []fileEntry{{}}
	return bridge
}

func (b *rawBridge) inode(id uint64, fh uint64) (*Inode, fileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nodes[id].inode, b.files[fh]
}

func (b *rawBridge) Lookup(header *fuse.InHeader, name string, out *fuse.EntryOut) (status fuse.Status) {
	parent, _ := b.inode(header.NodeId, 0)

	child, code := parent.node.Lookup(context.TODO(), name, out)
	if !code.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return code
	}

	lockNodes(parent, child)
	parent.setEntry(name, child)
	b.mu.Lock()
	if child.nodeID == 0 {
		b.registerInode(child)
	}
	out.NodeId = child.nodeID
	out.Generation = b.nodes[out.NodeId].generation
	b.mu.Unlock()
	unlockNodes(parent, child)

	if b.options.AttrTimeout != nil {
		out.SetAttrTimeout(*b.options.AttrTimeout)
	}
	if b.options.EntryTimeout != nil {
		out.SetEntryTimeout(*b.options.EntryTimeout)
	}

	return fuse.OK
}

// registerInode sets an nodeID in the child. Must have bridge.mu and
// child.mu
func (b *rawBridge) registerInode(child *Inode) {
	if l := len(b.free); l > 0 {
		last := b.free[l-1]
		b.free = b.free[:l-1]

		child.nodeID = last
		b.nodes[last].inode = child
		b.nodes[last].generation++
	} else {
		last := len(b.nodes)
		b.nodes = append(b.nodes, mapEntry{
			inode: child,
		})
		child.nodeID = uint64(last)
	}
}

func (b *rawBridge) Create(input *fuse.CreateIn, name string, out *fuse.CreateOut) (code fuse.Status) {
	ctx := context.TODO()
	parent, _ := b.inode(input.NodeId, 0)
	child, f, flags, code := parent.node.Create(ctx, name, input.Flags, input.Mode)
	if !code.Ok() {
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
		return code
	}

	lockNode2(parent, child)
	parent.setEntry(name, child)
	b.mu.Lock()
	if child.nodeID == 0 {
		b.registerInode(child)
	}
	out.Fh = b.registerFile(f)
	out.NodeId = child.nodeID
	out.Generation = b.nodes[child.nodeID].generation
	b.mu.Unlock()
	unlockNode2(parent, child)

	if b.options.AttrTimeout != nil {
		out.SetAttrTimeout(*b.options.AttrTimeout)
	}
	if b.options.EntryTimeout != nil {
		out.SetEntryTimeout(*b.options.EntryTimeout)
	}

	out.OpenFlags = flags

	f.GetAttr(ctx, &out.Attr)
	return fuse.OK
}

func (b *rawBridge) Forget(nodeid, nlookup uint64) {
	b.mu.Lock()
	n := b.nodes[nodeid].inode
	b.mu.Unlock()

	n.removeRef(nlookup, false)
}

func (b *rawBridge) unregisterNode(nodeid uint64) {
	b.free = append(b.free, nodeid)
	b.nodes[nodeid].inode = nil
}

func (b *rawBridge) SetDebug(debug bool) {}

func (b *rawBridge) GetAttr(input *fuse.GetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	n, fEntry := b.inode(input.NodeId, input.Fh())
	f := fEntry.file

	if input.Flags()&fuse.FUSE_GETATTR_FH == 0 {
		f = nil
	}

	dest := &out.Attr
	code = n.node.GetAttr(context.TODO(), f, dest)
	if out.Nlink == 0 {
		// With Nlink == 0, newer kernels will refuse link
		// operations.
		out.Nlink = 1
	}

	if b.options.AttrTimeout != nil {
		out.SetTimeout(*b.options.AttrTimeout)
	}
	return code
}

func (b *rawBridge) SetAttr(input *fuse.SetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	ctx := context.TODO()

	n, fEntry := b.inode(input.NodeId, input.Fh)
	f := fEntry.file
	if input.Valid&fuse.FATTR_FH == 0 {
		f = nil
	}

	if input.Valid&fuse.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = n.node.Chmod(ctx, f, permissions)
	}

	if code.Ok() && (input.Valid&(fuse.FATTR_UID|fuse.FATTR_GID) != 0) {
		var uid uint32 = ^uint32(0) // means "do not change" in chown(2)
		var gid uint32 = ^uint32(0)
		if input.Valid&fuse.FATTR_UID != 0 {
			uid = input.Uid
		}
		if input.Valid&fuse.FATTR_GID != 0 {
			gid = input.Gid
		}
		code = n.node.Chown(ctx, f, uid, gid)
	}

	if code.Ok() && input.Valid&fuse.FATTR_SIZE != 0 {
		code = n.node.Truncate(ctx, f, input.Size)
	}

	if code.Ok() && (input.Valid&(fuse.FATTR_ATIME|fuse.FATTR_MTIME|fuse.FATTR_ATIME_NOW|fuse.FATTR_MTIME_NOW) != 0) {
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

		code = n.node.Utimens(ctx, f, atime, mtime)
	}

	if !code.Ok() {
		return code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	attr := &out.Attr
	code = n.node.GetAttr(ctx, f, attr)

	// TODO - attr timout?
	return code
}

func (b *rawBridge) Mknod(input *fuse.MknodIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Mkdir(input *fuse.MkdirIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Unlink(header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Rmdir(header *fuse.InHeader, name string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Rename(input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Link(input *fuse.LinkIn, filename string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Symlink(header *fuse.InHeader, pointedTo string, linkName string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Readlink(header *fuse.InHeader) (out []byte, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (b *rawBridge) Access(input *fuse.AccessIn) (code fuse.Status) {
	return fuse.ENOSYS
}

// Extended attributes.
func (b *rawBridge) GetXAttrSize(header *fuse.InHeader, attr string) (sz int, code fuse.Status) {
	return 0, fuse.ENOSYS
}

func (b *rawBridge) GetXAttrData(header *fuse.InHeader, attr string) (data []byte, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (b *rawBridge) ListXAttr(header *fuse.InHeader) (attributes []byte, code fuse.Status) {
	return nil, fuse.ENOSYS
}

func (b *rawBridge) SetXAttr(input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	return fuse.ENOSYS
}

func (b *rawBridge) RemoveXAttr(header *fuse.InHeader, attr string) (code fuse.Status) {
	return
}

func (b *rawBridge) Open(input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	n, _ := b.inode(input.NodeId, 0)
	// NOSUBMIT: what about the mode argument?
	f, flags, code := n.node.Open(context.TODO(), input.Flags)
	if !code.Ok() {
		return code
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	out.Fh = b.registerFile(f)
	out.OpenFlags = flags
	return fuse.OK
}

// registerFile hands out a file handle. Must have bridge.mu
//
// XXX is it allowed to return the same Fh from two different Open
// calls on the same inode?
func (b *rawBridge) registerFile(f File) uint64 {
	var fh uint64
	if len(b.freeFiles) > 0 {
		last := uint64(len(b.freeFiles) - 1)
		fh = b.freeFiles[last]
		b.freeFiles = b.freeFiles[:last]
	} else {
		fh = uint64(len(b.files))
		b.files = append(b.files, fileEntry{})
	}

	b.files[fh].file = f
	return fh
}

func (b *rawBridge) Read(input *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Read(context.TODO(), f.file, buf, int64(input.Offset))
}

func (b *rawBridge) GetLk(input *fuse.LkIn, out *fuse.LkOut) (code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.GetLk(context.TODO(), f.file, input.Owner, &input.Lk, input.LkFlags, &out.Lk)
}

func (b *rawBridge) SetLk(input *fuse.LkIn) (code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.SetLk(context.TODO(), f.file, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) SetLkw(input *fuse.LkIn) (code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.SetLkw(context.TODO(), f.file, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) Release(input *fuse.ReleaseIn) {
	n, f := b.inode(input.NodeId, input.Fh)
	n.node.Release(context.TODO(), f.file)

	if input.Fh > 0 {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.files[input.Fh].file = nil
		b.freeFiles = append(b.freeFiles, input.Fh)
	}
}

func (b *rawBridge) Write(input *fuse.WriteIn, data []byte) (written uint32, code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Write(context.TODO(), f.file, data, int64(input.Offset))
}

func (b *rawBridge) Flush(input *fuse.FlushIn) fuse.Status {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Flush(context.TODO(), f.file)
}

func (b *rawBridge) Fsync(input *fuse.FsyncIn) (code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Fsync(context.TODO(), f.file, input.FsyncFlags)
}

func (b *rawBridge) Fallocate(input *fuse.FallocateIn) (code fuse.Status) {
	n, f := b.inode(input.NodeId, input.Fh)
	return n.node.Allocate(context.TODO(), f.file, input.Offset, input.Length, input.Mode)
}

func (b *rawBridge) OpenDir(input *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	return
}

func (b *rawBridge) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return fuse.ENOSYS
}

func (b *rawBridge) ReadDirPlus(input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return fuse.ENOSYS
}

func (b *rawBridge) ReleaseDir(input *fuse.ReleaseIn) {
	return
}

func (b *rawBridge) FsyncDir(input *fuse.FsyncIn) (code fuse.Status) {
	return
}

func (b *rawBridge) StatFs(input *fuse.InHeader, out *fuse.StatfsOut) (code fuse.Status) {
	return
}
func (b *rawBridge) Init(*fuse.Server) {
}
