// FileSystemConnector's implementation of RawFileSystem

package fuse

import (
	"bytes"
	"log"
	"os"
	"time"
)

var _ = log.Println


func (me *FileSystemConnector) Init(fsInit *RawFsInit) {
	me.fsInit = *fsInit
}

func (me *FileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, status Status) {
	parent := me.getInodeData(header.NodeId)
	out, status, _ = me.internalLookup(parent, name, 1, &header.Context)
	return out, status
}

func (me *FileSystemConnector) internalMountLookup(mount *fileSystemMount, lookupCount int) (out *EntryOut, status Status, node *inode) {
	fi, err := mount.fs.RootNode().GetAttr(nil, nil)
	if err == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	if !err.Ok() {
		return nil, err, mount.mountInode
	}
	mount.treeLock.Lock()
	defer mount.treeLock.Unlock()
	mount.mountInode.lookupCount += lookupCount
	out = &EntryOut{
		NodeId:     mount.mountInode.nodeId,
		Generation: 1, // where to get the generation?
	}
	mount.fileInfoToEntry(fi, out)
	return out, OK, mount.mountInode
}

func (me *FileSystemConnector) internalLookup(parent *inode, name string, lookupCount int, context *Context) (out *EntryOut, code Status, node *inode) {
	if mount := me.lookupMount(parent, name, lookupCount); mount != nil {
		return me.internalMountLookup(mount, lookupCount)
	}

	var fi *os.FileInfo
	child := parent.getChild(name)
	if child != nil {
		fi, code = child.fsInode.GetAttr(nil, nil)
	}
	mount := parent.mount
	if code == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	if !code.Ok() {
		return nil, code, nil
	}

	if child != nil && code.Ok() {
		out = &EntryOut{
			NodeId:     child.nodeId,
			Generation: 1, // where to get the generation?
		}
		parent.mount.fileInfoToEntry(fi, out)	
		return out, OK, child
	}
	
	fi, fsNode, code := parent.fsInode.Lookup(name)
	if code == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	if !code.Ok() {
		return nil, code, nil
	}

	out, _ =  me.createChild(parent, name, fi, fsNode)
	return out, OK, node
}

func (me *FileSystemConnector) Forget(h *InHeader, input *ForgetIn) {
	me.forgetUpdate(h.NodeId, int(input.Nlookup))
}

func (me *FileSystemConnector) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	node := me.getInodeData(header.NodeId)

	var f File
	if input.Flags&FUSE_GETATTR_FH != 0 {
		if opened := me.getOpenedFile(input.Fh); opened != nil {
			f = opened.file
		}
	}
	
	fi, code := node.fsInode.GetAttr(f, &header.Context)
	if !code.Ok() {
		return nil, code
	}
	out = &AttrOut{}
	out.Attr.Ino = header.NodeId
	node.mount.fileInfoToAttr(fi, out)
	return out, OK
}
	
func (me *FileSystemConnector) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, code Status) {
	node := me.getInodeData(header.NodeId)
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
	opened.FuseFlags |= FOPEN_NONSEEKABLE
	return opened.FuseFlags, h, OK
}

func (me *FileSystemConnector) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	opened := me.getOpenedFile(input.Fh)
	de, code := opened.dir.ReadDir(input)
	if code != OK {
		return nil, code
	}
	return de, OK
}

func (me *FileSystemConnector) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	node := me.getInodeData(header.NodeId)
	f, code := node.fsInode.Open(input.Flags, &header.Context)
	if !code.Ok() {
		return 0, 0, code
	}
	h, opened := node.mount.registerFileHandle(node, nil, f, input.Flags)
	return opened.FuseFlags, h, OK
}

func (me *FileSystemConnector) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	var f File
	if input.Valid&FATTR_FH != 0 {
		me.getOpenedFile(input.Fh)
	}

	node := me.getInodeData(header.NodeId)
	if code.Ok() && input.Valid&FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, &header.Context)
	}
	if code.Ok() && (input.Valid&(FATTR_UID|FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), &header.Context)
	}
	if code.Ok() && input.Valid&FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, &header.Context)
	}
	if code.Ok() && (input.Valid&(FATTR_ATIME|FATTR_MTIME|FATTR_ATIME_NOW|FATTR_MTIME_NOW) != 0) {
		atime := uint64(input.Atime*1e9) + uint64(input.Atimensec)
		if input.Valid&FATTR_ATIME_NOW != 0 {
			atime = uint64(time.Nanoseconds())
		}

		mtime := uint64(input.Mtime*1e9) + uint64(input.Mtimensec)
		if input.Valid&FATTR_MTIME_NOW != 0 {
			mtime = uint64(time.Nanoseconds())
		}

		// TODO - if using NOW, mtime and atime may differ.
		code = node.fsInode.Utimens(f, atime, mtime, &header.Context)
	}

	if !code.Ok() {
		return nil, code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	fi, code := node.fsInode.GetAttr(f, &header.Context)
	out = &AttrOut{}
	out.Attr.Ino = header.NodeId
	node.mount.fileInfoToAttr(fi, out)
	return out, code
}

func (me *FileSystemConnector) Readlink(header *InHeader) (out []byte, code Status) {
	n := me.getInodeData(header.NodeId)
	return n.fsInode.Readlink(&header.Context)
}

func (me *FileSystemConnector) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	fi, fsNode, code := parent.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), &header.Context)
	if code.Ok() {
		out, _ = me.createChild(parent, name, fi, fsNode)
	}
	return out, code
}

func (me *FileSystemConnector) createChild(parent *inode, name string, fi *os.FileInfo, fsi *fsInode) (out *EntryOut, child *inode) {
	child = parent.createChild(name, fi.IsDirectory(), fsi, me)
	out = &EntryOut{}
	parent.mount.fileInfoToEntry(fi, out)
	out.Ino = child.nodeId
	out.NodeId = child.nodeId
	return out, child
}


func (me *FileSystemConnector) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	fi, fsInode, code := parent.fsInode.Mkdir(name, input.Mode, &header.Context)

	if code.Ok() {
		out, _ = me.createChild(parent, name, fi, fsInode)
	}
	return out, code
}

func (me *FileSystemConnector) Unlink(header *InHeader, name string) (code Status) {
	parent := me.getInodeData(header.NodeId)
	code = parent.fsInode.Unlink(name, &header.Context)
	if code.Ok() {
		// Like fuse.c, we update our internal tables.
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *FileSystemConnector) Rmdir(header *InHeader, name string) (code Status) {
	parent := me.getInodeData(header.NodeId)
	code = parent.fsInode.Rmdir(name, &header.Context)
	if code.Ok() {
		// Like fuse.c, we update our internal tables.
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *FileSystemConnector) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	fi, fsNode, code := parent.fsInode.Symlink(linkName, pointedTo, &header.Context)
	if code.Ok() {
		out, _ = me.createChild(parent, linkName, fi, fsNode)
	}
	return out, code
}


func (me *FileSystemConnector) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	oldParent := me.getInodeData(header.NodeId)
	isMountPoint := me.lookupMount(oldParent, oldName, 0) != nil
	if isMountPoint {
		return EBUSY
	}

	newParent := me.getInodeData(input.Newdir)
	if oldParent.mount != newParent.mount {
		return EXDEV
	}

	code = oldParent.fsInode.Rename(oldName, newParent.fsInode, newName, &header.Context)
	if code.Ok() {
		me.renameUpdate(oldParent, oldName, newParent, newName)
	}
	return code
}

func (me *FileSystemConnector) Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status) {
	existing := me.getInodeData(input.Oldnodeid)
	parent := me.getInodeData(header.NodeId)

	if existing.mount != parent.mount {
		return nil, EXDEV
	}

	code = parent.fsInode.Link(filename, existing.fsInode, &header.Context)
	if !code.Ok() {
		return nil, code
	}
	// TODO - revise this for real hardlinks?
	out, code, _ = me.internalLookup(parent, filename, 1, &header.Context)
	return out, code
}

func (me *FileSystemConnector) Access(header *InHeader, input *AccessIn) (code Status) {
	n := me.getInodeData(header.NodeId)
	return n.fsInode.Access(input.Mask, &header.Context)
}

func (me *FileSystemConnector) Create(header *InHeader, input *CreateIn, name string) (flags uint32, h uint64, out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	f, fi, fsNode, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, &header.Context)
	if !code.Ok() {
		return 0, 0, nil, code
	}
	out, child := me.createChild(parent, name, fi, fsNode)
	handle, opened := parent.mount.registerFileHandle(child, nil, f, input.Flags)
	return opened.FuseFlags, handle, out, code
}

func (me *FileSystemConnector) Release(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	node.mount.unregisterFileHandle(input.Fh, node)
}

func (me *FileSystemConnector) Flush(header *InHeader, input *FlushIn) Status {
	node := me.getInodeData(header.NodeId)
	opened := me.getOpenedFile(input.Fh)
	return node.fsInode.Flush(opened.file, opened.OpenFlags, &header.Context)
}

func (me *FileSystemConnector) ReleaseDir(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.dir.Release()
	me.considerDropInode(node)
}

func (me *FileSystemConnector) GetXAttr(header *InHeader, attribute string) (data []byte, code Status) {
	node := me.getInodeData(header.NodeId)
	return node.fsInode.GetXAttr(attribute, &header.Context)
}

func (me *FileSystemConnector) RemoveXAttr(header *InHeader, attr string) Status {
	node := me.getInodeData(header.NodeId)
	return node.fsInode.RemoveXAttr(attr, &header.Context)
}	

func (me *FileSystemConnector) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	node := me.getInodeData(header.NodeId)
	return node.fsInode.SetXAttr(attr, data, int(input.Flags), &header.Context)
}

func (me *FileSystemConnector) ListXAttr(header *InHeader) (data []byte, code Status) {
	node := me.getInodeData(header.NodeId)
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

func (me *FileSystemConnector) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	opened := me.getOpenedFile(input.Fh)
	return opened.file.Write(input, data)
}

func (me *FileSystemConnector) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	opened := me.getOpenedFile(input.Fh)
	return opened.file.Read(input, bp)
}

func (me *FileSystemConnector) StatFs() *StatfsOut {
	return me.rootNode.mountPoint.fs.StatFs()
}
