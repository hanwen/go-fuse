// FileSystemConnector's implementation of RawFileSystem

package fuse

import (
	"bytes"
	"log"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

func (me *FileSystemConnector) Init(fsInit *RawFsInit) {
	me.fsInit = *fsInit
}

func (me *FileSystemConnector) lookupMountUpdate(mount *fileSystemMount) (fi *Attr, node *Inode, code Status) {
	fi, code = mount.fs.Root().GetAttr(nil, nil)
	if !code.Ok() {
		log.Println("Root getattr should not return error", code)
		return &Attr{Mode: S_IFDIR | 0755}, mount.mountInode, OK
	}

	return fi, mount.mountInode, OK
}

func (me *FileSystemConnector) internalLookup(parent *Inode, name string, context *Context) (fi *Attr, node *Inode, code Status) {
	if subMount := me.findMount(parent, name); subMount != nil {
		return me.lookupMountUpdate(subMount)
	}

	child := parent.GetChild(name)
	if child != nil {
		parent = nil
	}
	var fsNode FsNode
	if child != nil {
		fi, code = child.fsInode.GetAttr(nil, context)
		fsNode = child.FsNode()
	} else {
		fi, fsNode, code = parent.fsInode.Lookup(name, context)
	}

	if child == nil && fsNode != nil {
		child = fsNode.Inode()
	}

	return fi, child, code
}

func (me *FileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, code Status) {
	parent := me.toInode(header.NodeId)
	if !parent.IsDir() {
		log.Printf("Lookup %q called on non-Directory node %d", name, header.NodeId)
		return nil, ENOTDIR
	}
	context := &header.Context
	fi, child, code := me.internalLookup(parent, name, context)
	if !code.Ok() {
		if code == ENOENT {
			return parent.mount.negativeEntry()
		}
		return nil, code
	}
	if child == nil {
		log.Println("HUH", name)
	}
	out = child.mount.fileInfoToEntry(fi)
	out.NodeId = me.lookupUpdate(child)
	out.Generation = 1
	out.Ino = out.NodeId

	return out, OK
}

func (me *FileSystemConnector) Forget(nodeID, nlookup uint64) {
	node := me.toInode(nodeID)
	me.forgetUpdate(node, int(nlookup))
}

func (me *FileSystemConnector) GetAttr(header *InHeader, input *raw.GetAttrIn) (out *AttrOut, code Status) {
	node := me.toInode(header.NodeId)

	var f File
	if input.Flags&raw.FUSE_GETATTR_FH != 0 {
		if opened := node.mount.getOpenedFile(input.Fh); opened != nil {
			f = opened.WithFlags.File
		}
	}

	fi, code := node.fsInode.GetAttr(f, &header.Context)
	if !code.Ok() {
		return nil, code
	}
	out = node.mount.fillAttr(fi, header.NodeId)
	return out, OK
}

func (me *FileSystemConnector) OpenDir(header *InHeader, input *raw.OpenIn) (flags uint32, handle uint64, code Status) {
	node := me.toInode(header.NodeId)
	stream, err := node.fsInode.OpenDir(&header.Context)
	if err != OK {
		return 0, 0, err
	}

	de := &connectorDir{
		extra:  node.getMountDirEntries(),
		stream: stream,
	}
	de.extra = append(de.extra, DirEntry{S_IFDIR, "."}, DirEntry{S_IFDIR, ".."})
	h, opened := node.mount.registerFileHandle(node, de, nil, input.Flags)

	// TODO - implement seekable directories
	opened.FuseFlags |= raw.FOPEN_NONSEEKABLE
	return opened.FuseFlags, h, OK
}

func (me *FileSystemConnector) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	node := me.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	de, code := opened.dir.ReadDir(input)
	if code != OK {
		return nil, code
	}
	return de, OK
}

func (me *FileSystemConnector) Open(header *InHeader, input *raw.OpenIn) (flags uint32, handle uint64, status Status) {
	node := me.toInode(header.NodeId)
	f, code := node.fsInode.Open(input.Flags, &header.Context)
	if !code.Ok() {
		return 0, 0, code
	}
	h, opened := node.mount.registerFileHandle(node, nil, f, input.Flags)
	return opened.FuseFlags, h, OK
}

func (me *FileSystemConnector) SetAttr(header *InHeader, input *raw.SetAttrIn) (out *AttrOut, code Status) {
	node := me.toInode(header.NodeId)
	var f File
	if input.Valid&raw.FATTR_FH != 0 {
		opened := node.mount.getOpenedFile(input.Fh)
		f = opened.WithFlags.File
	}

	if code.Ok() && input.Valid&raw.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, &header.Context)
	}
	if code.Ok() && (input.Valid&(raw.FATTR_UID|raw.FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), &header.Context)
	}
	if code.Ok() && input.Valid&raw.FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, &header.Context)
	}
	if code.Ok() && (input.Valid&(raw.FATTR_ATIME|raw.FATTR_MTIME|raw.FATTR_ATIME_NOW|raw.FATTR_MTIME_NOW) != 0) {
		now := int64(0)
		if input.Valid&raw.FATTR_ATIME_NOW != 0 || input.Valid&raw.FATTR_MTIME_NOW != 0 {
			now = time.Now().UnixNano()
		}

		atime := int64(input.Atime*1e9) + int64(input.Atimensec)
		if input.Valid&raw.FATTR_ATIME_NOW != 0 {
			atime = now
		}

		mtime := int64(input.Mtime*1e9) + int64(input.Mtimensec)
		if input.Valid&raw.FATTR_MTIME_NOW != 0 {
			mtime = now
		}

		code = node.fsInode.Utimens(f, atime, mtime, &header.Context)
	}

	if !code.Ok() {
		return nil, code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	fi, code := node.fsInode.GetAttr(f, &header.Context)

	if code.Ok() {
		out = node.mount.fillAttr(fi, header.NodeId)
	}
	return out, code
}

func (me *FileSystemConnector) Readlink(header *InHeader) (out []byte, code Status) {
	n := me.toInode(header.NodeId)
	return n.fsInode.Readlink(&header.Context)
}

func (me *FileSystemConnector) Mknod(header *InHeader, input *raw.MknodIn, name string) (out *EntryOut, code Status) {
	parent := me.toInode(header.NodeId)
	fi, fsNode, code := parent.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), &header.Context)
	if code.Ok() {
		out = me.childLookup(fi, fsNode)
	}
	return out, code
}

func (me *FileSystemConnector) Mkdir(header *InHeader, input *raw.MkdirIn, name string) (out *EntryOut, code Status) {
	parent := me.toInode(header.NodeId)
	fi, fsInode, code := parent.fsInode.Mkdir(name, input.Mode, &header.Context)

	if code.Ok() {
		out = me.childLookup(fi, fsInode)
	}
	return out, code
}

func (me *FileSystemConnector) Unlink(header *InHeader, name string) (code Status) {
	parent := me.toInode(header.NodeId)
	return parent.fsInode.Unlink(name, &header.Context)
}

func (me *FileSystemConnector) Rmdir(header *InHeader, name string) (code Status) {
	parent := me.toInode(header.NodeId)
	return parent.fsInode.Rmdir(name, &header.Context)
}

func (me *FileSystemConnector) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	parent := me.toInode(header.NodeId)
	fi, fsNode, code := parent.fsInode.Symlink(linkName, pointedTo, &header.Context)
	if code.Ok() {
		out = me.childLookup(fi, fsNode)
	}
	return out, code
}

func (me *FileSystemConnector) Rename(header *InHeader, input *raw.RenameIn, oldName string, newName string) (code Status) {
	oldParent := me.toInode(header.NodeId)
	isMountPoint := me.findMount(oldParent, oldName) != nil
	if isMountPoint {
		return EBUSY
	}

	newParent := me.toInode(input.Newdir)
	if oldParent.mount != newParent.mount {
		return EXDEV
	}

	return oldParent.fsInode.Rename(oldName, newParent.fsInode, newName, &header.Context)
}

func (me *FileSystemConnector) Link(header *InHeader, input *raw.LinkIn, name string) (out *EntryOut, code Status) {
	existing := me.toInode(input.Oldnodeid)
	parent := me.toInode(header.NodeId)

	if existing.mount != parent.mount {
		return nil, EXDEV
	}

	fi, fsInode, code := parent.fsInode.Link(name, existing.fsInode, &header.Context)
	if code.Ok() {
		out = me.childLookup(fi, fsInode)
	}

	return out, code
}

func (me *FileSystemConnector) Access(header *InHeader, input *raw.AccessIn) (code Status) {
	n := me.toInode(header.NodeId)
	return n.fsInode.Access(input.Mask, &header.Context)
}

func (me *FileSystemConnector) Create(header *InHeader, input *raw.CreateIn, name string) (flags uint32, h uint64, out *EntryOut, code Status) {
	parent := me.toInode(header.NodeId)
	f, fi, fsNode, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, &header.Context)
	if !code.Ok() {
		return 0, 0, nil, code
	}
	out = me.childLookup(fi, fsNode)
	handle, opened := parent.mount.registerFileHandle(fsNode.Inode(), nil, f, input.Flags)
	return opened.FuseFlags, handle, out, code
}

func (me *FileSystemConnector) Release(header *InHeader, input *raw.ReleaseIn) {
	node := me.toInode(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.WithFlags.File.Release()
}

func (me *FileSystemConnector) ReleaseDir(header *InHeader, input *raw.ReleaseIn) {
	node := me.toInode(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.dir.Release()
	me.considerDropInode(node)
}

func (me *FileSystemConnector) GetXAttrSize(header *InHeader, attribute string) (sz int, code Status) {
	node := me.toInode(header.NodeId)
	data, c := node.fsInode.GetXAttr(attribute, &header.Context)
	return len(data), c
}

func (me *FileSystemConnector) GetXAttrData(header *InHeader, attribute string) (data []byte, code Status) {
	node := me.toInode(header.NodeId)
	return node.fsInode.GetXAttr(attribute, &header.Context)
}

func (me *FileSystemConnector) RemoveXAttr(header *InHeader, attr string) Status {
	node := me.toInode(header.NodeId)
	return node.fsInode.RemoveXAttr(attr, &header.Context)
}

func (me *FileSystemConnector) SetXAttr(header *InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status {
	node := me.toInode(header.NodeId)
	return node.fsInode.SetXAttr(attr, data, int(input.Flags), &header.Context)
}

func (me *FileSystemConnector) ListXAttr(header *InHeader) (data []byte, code Status) {
	node := me.toInode(header.NodeId)
	attrs, code := node.fsInode.ListXAttr(&header.Context)
	if code != OK {
		return nil, code
	}

	b := bytes.NewBuffer([]byte{})
	for _, v := range attrs {
		b.Write([]byte(v))
		b.WriteByte(0)
	}

	return b.Bytes(), code
}

////////////////
// files.

func (me *FileSystemConnector) Write(header *InHeader, input *WriteIn, data []byte) (written uint32, code Status) {
	node := me.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Write(input, data)
}

func (me *FileSystemConnector) Read(header *InHeader, input *ReadIn, bp BufferPool) ([]byte, Status) {
	node := me.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Read(input, bp)
}

func (me *FileSystemConnector) StatFs(header *InHeader) *StatfsOut {
	node := me.toInode(header.NodeId)
	return node.FsNode().StatFs()
}

func (me *FileSystemConnector) Flush(header *InHeader, input *raw.FlushIn) Status {
	node := me.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Flush()
}
