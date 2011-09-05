// Translation of raw operation to path based operations.

package fuse

import (
	"bytes"
	"fmt"
	"log"
	"time"
)

var _ = log.Println

func NewFileSystemConnector(fs FileSystem, opts *FileSystemOptions) (me *FileSystemConnector) {
	me = new(FileSystemConnector)
	if opts == nil {
		opts = NewFileSystemOptions()
	}
	me.inodeMap = NewHandleMap(!opts.SkipCheckHandles)
	me.rootNode = me.newInode(true)
	me.rootNode.NodeId = FUSE_ROOT_ID
	me.verify()
	me.mountRoot(fs, opts)
	return me
}

func (me *FileSystemConnector) GetPath(nodeid uint64) (path string, mount *fileSystemMount, node *inode) {
	n := me.getInodeData(nodeid)

	p := n.fsInode.GetPath()
	return p, n.mount, n
}

func (me *fileSystemMount) setOwner(attr *Attr) {
	if me.options.Owner != nil {
		attr.Owner = *me.options.Owner
	}
}

func (me *FileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, status Status) {
	parent := me.getInodeData(header.NodeId)
	return me.internalLookup(parent, name, 1, &header.Context)
}

func (me *FileSystemConnector) internalLookup(parent *inode, name string, lookupCount int, context *Context) (out *EntryOut, status Status) {
	out, status, _ = me.internalLookupWithNode(parent, name, lookupCount, context)
	return out, status
}

func (me *FileSystemConnector) internalMountLookup(mount *fileSystemMount, lookupCount int) (out *EntryOut, status Status, node *inode) {
	fi, err := mount.fs.GetAttr("", nil)
	if err == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	if !err.Ok() {
		return nil, err, mount.mountInode
	}
	mount.treeLock.Lock()
	defer mount.treeLock.Unlock()
	mount.mountInode.LookupCount += lookupCount
	out = &EntryOut{
		NodeId:     mount.mountInode.NodeId,
		Generation: 1, // where to get the generation?
	}
	mount.fileInfoToEntry(fi, out)
	return out, OK, mount.mountInode
}

func (me *FileSystemConnector) internalLookupWithNode(parent *inode, name string, lookupCount int, context *Context) (out *EntryOut, status Status, node *inode) {
	if mount := me.lookupMount(parent, name, lookupCount); mount != nil {
		return me.internalMountLookup(mount, lookupCount)
	}

	fi, code := parent.fsInode.Lookup(name)
	mount := parent.mount
	if code == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	if !code.Ok() {
		return nil, code, nil
	}
	node = me.lookupUpdate(parent, name, fi.IsDirectory(), lookupCount)
	out = &EntryOut{
		NodeId:     node.NodeId,
		Generation: 1, // where to get the generation?
	}
	parent.mount.fileInfoToEntry(fi, out)
	out.Attr.Ino = node.NodeId
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
	fi, code := node.fsInode.GetAttr(f, &header.Context)
	
	if code.Ok() && input.Valid&FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, &header.Context)
		fi.Mode = (fi.Mode &^ 07777) | permissions
	}
	if code.Ok() && (input.Valid&(FATTR_UID|FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), &header.Context)
		fi.Uid = int(input.Uid)
		fi.Gid = int(input.Gid)
	}
	if code.Ok() && input.Valid&FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, &header.Context)
		fi.Size = int64(input.Size)
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
		fi.Atime_ns = int64(atime)
		fi.Mtime_ns = int64(mtime)
	}

	if !code.Ok() {
		return nil, code
	}

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
	n := me.getInodeData(header.NodeId)
	code = n.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), &header.Context)
	if code.Ok() {
		return me.internalLookup(n, name, 1, &header.Context)
	}
	return nil, code
}

func (me *FileSystemConnector) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	code = parent.fsInode.Mkdir(name, input.Mode, &header.Context)

	if code.Ok() {
		out, code = me.internalLookup(parent, name, 1, &header.Context)
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
	code = parent.fsInode.Symlink(linkName, pointedTo, &header.Context)
	if code.Ok() {
		return me.internalLookup(parent, linkName, 1, &header.Context)
	}
	return nil, code
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
	return me.internalLookup(parent, filename, 1, &header.Context)
}

func (me *FileSystemConnector) Access(header *InHeader, input *AccessIn) (code Status) {
	n := me.getInodeData(header.NodeId)
	return n.fsInode.Access(input.Mask, &header.Context)
}

func (me *FileSystemConnector) Create(header *InHeader, input *CreateIn, name string) (flags uint32, h uint64, out *EntryOut, code Status) {
	parent := me.getInodeData(header.NodeId)
	f, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, &header.Context)
	if !code.Ok() {
		return 0, 0, nil, code
	}

	out, code, inode := me.internalLookupWithNode(parent, name, 1, &header.Context)
	if inode == nil {
		msg := fmt.Sprintf("Create succeded, but GetAttr returned no entry %v, %q. code %v", header.NodeId, name, code)
		panic(msg)
	}
	handle, opened := parent.mount.registerFileHandle(inode, nil, f, input.Flags)
	return opened.FuseFlags, handle, out, code
}

func (me *FileSystemConnector) Release(header *InHeader, input *ReleaseIn) {
	opened := me.getOpenedFile(input.Fh)
	opened.inode.mount.unregisterFileHandle(input.Fh)
}

func (me *FileSystemConnector) Flush(header *InHeader, input *FlushIn) Status {
	opened := me.getOpenedFile(input.Fh)
	return opened.inode.fsInode.Flush(opened.file, opened.OpenFlags, &header.Context)
}

func (me *FileSystemConnector) ReleaseDir(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh)
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
