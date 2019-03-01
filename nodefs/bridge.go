// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"golang.org/x/sys/unix"
)

type fileEntry struct {
	file FileHandle

	// space to hold directory stuff
}

type rawBridge struct {
	fuse.RawFileSystem

	options Options
	root    *Inode

	// mu protects the following data.  Locks for inodes must be
	// taken before rawBridge.mu
	mu           sync.Mutex
	nodes        map[uint64]*Inode
	automaticIno uint64

	files     []fileEntry
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
	return inode
}

func NewNodeFS(root Operations, opts *Options) fuse.RawFileSystem {
	bridge := &rawBridge{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		automaticIno:  1 << 63,
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
	bridge.files = []fileEntry{{}}
	return bridge
}

func (b *rawBridge) inode(id uint64, fh uint64) (*Inode, fileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, f := b.nodes[id], b.files[fh]
	if n == nil {
		log.Panicf("unknown node %d", id)
	}
	if fh != 0 && f.file == nil {
		log.Panicf("unknown fh %d", fh)
	}
	return n, f
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

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)

	out.Mode = child.mode | (out.Mode & 07777)
	return fuse.OK
}

func (b *rawBridge) Rmdir(header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	code := parent.node.Rmdir(context.TODO(), name)
	if code.Ok() {
		parent.RmChild(name)
	}
	return code

}

func (b *rawBridge) Unlink(header *fuse.InHeader, name string) fuse.Status {
	parent, _ := b.inode(header.NodeId, 0)
	code := parent.node.Unlink(context.TODO(), name)
	if code.Ok() {
		parent.RmChild(name)
	}
	return code
}

func (b *rawBridge) Mkdir(input *fuse.MkdirIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)

	child, code := parent.node.Mkdir(context.TODO(), name, input.Mode, out)
	if !code.Ok() {
		return code
	}

	if out.Attr.Mode&^07777 != fuse.S_IFDIR {
		log.Panicf("Mkdir: mode must be S_IFDIR (%o), got %o", fuse.S_IFDIR, out.Attr.Mode)
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Mknod(input *fuse.MknodIn, name string, out *fuse.EntryOut) (code fuse.Status) {
	parent, _ := b.inode(input.NodeId, 0)

	child, code := parent.node.Mknod(context.TODO(), name, input.Mode, input.Rdev, out)
	if !code.Ok() {
		return code
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

func (b *rawBridge) GetAttr(input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	n, fEntry := b.inode(input.NodeId, input.Fh())
	f := fEntry.file

	if input.Flags()&fuse.FUSE_GETATTR_FH == 0 {
		f = nil
	}

	code := n.node.GetAttr(context.TODO(), f, out)
	b.setAttrTimeout(out)
	out.Ino = input.NodeId
	out.Mode = (out.Attr.Mode & 07777) | n.mode
	return code
}

func (b *rawBridge) setAttrTimeout(out *fuse.AttrOut) {
	if b.options.AttrTimeout != nil {
		out.SetTimeout(*b.options.AttrTimeout)
	}
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
	code = n.node.GetAttr(ctx, f, out)
	b.setAttrTimeout(out)
	out.Ino = n.nodeID.Ino
	out.Mode = n.mode | (out.Mode &^ 07777)
	return code
}

func (b *rawBridge) Rename(input *fuse.RenameIn, oldName string, newName string) fuse.Status {
	p1, _ := b.inode(input.NodeId, 0)
	p2, _ := b.inode(input.Newdir, 0)

	code := p1.node.Rename(context.TODO(), oldName, p2.node, newName, input.Flags)
	if code.Ok() {
		if input.Flags&unix.RENAME_EXCHANGE != 0 {
			// XXX - test coverage.
			p1.ExchangeChild(oldName, p2, newName)
		} else {
			p1.MvChild(oldName, p2, newName, true)
		}
	}
	return code
}

func (b *rawBridge) Link(input *fuse.LinkIn, filename string, out *fuse.EntryOut) (code fuse.Status) {
	return fuse.ENOSYS
}

func (b *rawBridge) Symlink(header *fuse.InHeader, target string, name string, out *fuse.EntryOut) (code fuse.Status) {
	log.Println("symlink1")
	parent, _ := b.inode(header.NodeId, 0)
	child, code := parent.node.Symlink(context.TODO(), target, name, out)
	if !code.Ok() {
		return code
	}

	b.addNewChild(parent, name, child, nil, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Readlink(header *fuse.InHeader) (out []byte, code fuse.Status) {
	n, _ := b.inode(header.NodeId, 0)
	result, code := n.node.Readlink(context.TODO())
	if !code.Ok() {
		return nil, code
	}

	return []byte(result), fuse.OK
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
func (b *rawBridge) registerFile(f FileHandle) uint64 {
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
