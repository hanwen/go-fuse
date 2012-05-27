// FileSystemConnector's implementation of RawFileSystem

package fuse

import (
	"bytes"
	"log"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

func (c *FileSystemConnector) Init(fsInit *RawFsInit) {
	c.fsInit = *fsInit
}

func (c *FileSystemConnector) lookupMountUpdate(out *Attr, mount *fileSystemMount) (node *Inode, code Status) {
	code = mount.fs.Root().GetAttr(out, nil, nil)
	if !code.Ok() {
		log.Println("Root getattr should not return error", code)
		out.Mode = S_IFDIR | 0755
		return mount.mountInode, OK
	}

	return mount.mountInode, OK
}

func (c *FileSystemConnector) internalLookup(out *Attr, parent *Inode, name string, context *Context) (node *Inode, code Status) {
	if subMount := c.findMount(parent, name); subMount != nil {
		return c.lookupMountUpdate(out, subMount)
	}

	child := parent.GetChild(name)
	if child != nil {
		parent = nil
	}
	var fsNode FsNode
	if child != nil {
		code = child.fsInode.GetAttr(out, nil, context)
		fsNode = child.FsNode()
	} else {
		fsNode, code = parent.fsInode.Lookup(out, name, context)
	}

	if child == nil && fsNode != nil {
		child = fsNode.Inode()
		if child == nil {
			log.Panicf("Lookup %q returned child without Inode: %v", name, fsNode)
		}
	}

	return child, code
}

func (c *FileSystemConnector) Lookup(out *raw.EntryOut, header *raw.InHeader, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	if !parent.IsDir() {
		log.Printf("Lookup %q called on non-Directory node %d", name, header.NodeId)
		return ENOTDIR
	}
	context := (*Context)(&header.Context)
	outAttr := (*Attr)(&out.Attr)
	child, code := c.internalLookup(outAttr, parent, name, context)
	if code == ENOENT && parent.mount.negativeEntry(out) {
		return OK
	}
	if !code.Ok() {
		return code
	}
	if child == nil {
		log.Println("Lookup returned OK with nil child", name)
	}

	child.mount.fillEntry(out)
	out.NodeId = c.lookupUpdate(child)
	out.Generation = 1
	out.Ino = out.NodeId

	return OK
}

func (c *FileSystemConnector) Forget(nodeID, nlookup uint64) {
	node := c.toInode(nodeID)
	c.forgetUpdate(node, int(nlookup))
}

func (c *FileSystemConnector) GetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.GetAttrIn) (code Status) {
	node := c.toInode(header.NodeId)

	var f File
	if input.Flags&raw.FUSE_GETATTR_FH != 0 {
		if opened := node.mount.getOpenedFile(input.Fh); opened != nil {
			f = opened.WithFlags.File
		}
	}

	dest := (*Attr)(&out.Attr)
	code = node.fsInode.GetAttr(dest, f, (*Context)(&header.Context))
	if !code.Ok() {
		return code
	}

	node.mount.fillAttr(out, header.NodeId)
	return OK
}

func (c *FileSystemConnector) OpenDir(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (code Status) {
	node := c.toInode(header.NodeId)
	stream, err := node.fsInode.OpenDir((*Context)(&header.Context))
	if err != OK {
		return err
	}
	stream = append(stream, node.getMountDirEntries()...)
	de := &connectorDir{
		node:   node.FsNode(),
		stream: append(stream, DirEntry{S_IFDIR, "."}, DirEntry{S_IFDIR, ".."}),
	}
	h, opened := node.mount.registerFileHandle(node, de, nil, input.Flags)
	out.OpenFlags = opened.FuseFlags
	out.Fh = h
	return OK
}

func (c *FileSystemConnector) ReadDir(l *DirEntryList, header *raw.InHeader, input *raw.ReadIn) Status {
	node := c.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.dir.ReadDir(l, input)
}

func (c *FileSystemConnector) Open(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status) {
	node := c.toInode(header.NodeId)
	f, code := node.fsInode.Open(input.Flags, (*Context)(&header.Context))
	if !code.Ok() {
		return code
	}
	h, opened := node.mount.registerFileHandle(node, nil, f, input.Flags)
	out.OpenFlags = opened.FuseFlags
	out.Fh = h
	return OK
}

func (c *FileSystemConnector) SetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.SetAttrIn) (code Status) {
	node := c.toInode(header.NodeId)
	var f File
	if input.Valid&raw.FATTR_FH != 0 {
		opened := node.mount.getOpenedFile(input.Fh)
		f = opened.WithFlags.File
	}

	if code.Ok() && input.Valid&raw.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, (*Context)(&header.Context))
	}
	if code.Ok() && (input.Valid&(raw.FATTR_UID|raw.FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), (*Context)(&header.Context))
	}
	if code.Ok() && input.Valid&raw.FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, (*Context)(&header.Context))
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

		code = node.fsInode.Utimens(f, atime, mtime, (*Context)(&header.Context))
	}

	if !code.Ok() {
		return code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	attr := (*Attr)(&out.Attr)
	code = node.fsInode.GetAttr(attr, nil, (*Context)(&header.Context))
	if code.Ok() {
		node.mount.fillAttr(out, header.NodeId)
	}
	return code
}

func (c *FileSystemConnector) Readlink(header *raw.InHeader) (out []byte, code Status) {
	n := c.toInode(header.NodeId)
	return n.fsInode.Readlink((*Context)(&header.Context))
}

func (c *FileSystemConnector) Mknod(out *raw.EntryOut, header *raw.InHeader, input *raw.MknodIn, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	ctx := (*Context)(&header.Context)
	fsNode, code := parent.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Mkdir(out *raw.EntryOut, header *raw.InHeader, input *raw.MkdirIn, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	ctx := (*Context)(&header.Context)
	fsNode, code := parent.fsInode.Mkdir(name, input.Mode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Unlink(header *raw.InHeader, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	return parent.fsInode.Unlink(name, (*Context)(&header.Context))
}

func (c *FileSystemConnector) Rmdir(header *raw.InHeader, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	return parent.fsInode.Rmdir(name, (*Context)(&header.Context))
}

func (c *FileSystemConnector) Symlink(out *raw.EntryOut, header *raw.InHeader, pointedTo string, linkName string) (code Status) {
	parent := c.toInode(header.NodeId)
	ctx := (*Context)(&header.Context)
	fsNode, code := parent.fsInode.Symlink(linkName, pointedTo, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Rename(header *raw.InHeader, input *raw.RenameIn, oldName string, newName string) (code Status) {
	oldParent := c.toInode(header.NodeId)
	isMountPoint := c.findMount(oldParent, oldName) != nil
	if isMountPoint {
		return EBUSY
	}

	newParent := c.toInode(input.Newdir)
	if oldParent.mount != newParent.mount {
		return EXDEV
	}

	return oldParent.fsInode.Rename(oldName, newParent.fsInode, newName, (*Context)(&header.Context))
}

func (c *FileSystemConnector) Link(out *raw.EntryOut, header *raw.InHeader, input *raw.LinkIn, name string) (code Status) {
	existing := c.toInode(input.Oldnodeid)
	parent := c.toInode(header.NodeId)

	if existing.mount != parent.mount {
		return EXDEV
	}
	ctx := (*Context)(&header.Context)
	fsNode, code := parent.fsInode.Link(name, existing.fsInode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}

	return code
}

func (c *FileSystemConnector) Access(header *raw.InHeader, input *raw.AccessIn) (code Status) {
	n := c.toInode(header.NodeId)
	return n.fsInode.Access(input.Mask, (*Context)(&header.Context))
}

func (c *FileSystemConnector) Create(out *raw.CreateOut, header *raw.InHeader, input *raw.CreateIn, name string) (code Status) {
	parent := c.toInode(header.NodeId)
	f, fsNode, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, (*Context)(&header.Context))
	if !code.Ok() {
		return code
	}

	c.childLookup(&out.EntryOut, fsNode)
	handle, opened := parent.mount.registerFileHandle(fsNode.Inode(), nil, f, input.Flags)

	out.OpenOut.OpenFlags = opened.FuseFlags
	out.OpenOut.Fh = handle
	return code
}

func (c *FileSystemConnector) Release(header *raw.InHeader, input *raw.ReleaseIn) {
	node := c.toInode(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.WithFlags.File.Release()
}

func (c *FileSystemConnector) ReleaseDir(header *raw.InHeader, input *raw.ReleaseIn) {
	node := c.toInode(header.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.dir.Release()
}

func (c *FileSystemConnector) GetXAttrSize(header *raw.InHeader, attribute string) (sz int, code Status) {
	node := c.toInode(header.NodeId)
	data, errno := node.fsInode.GetXAttr(attribute, (*Context)(&header.Context))
	return len(data), errno
}

func (c *FileSystemConnector) GetXAttrData(header *raw.InHeader, attribute string) (data []byte, code Status) {
	node := c.toInode(header.NodeId)
	return node.fsInode.GetXAttr(attribute, (*Context)(&header.Context))
}

func (c *FileSystemConnector) RemoveXAttr(header *raw.InHeader, attr string) Status {
	node := c.toInode(header.NodeId)
	return node.fsInode.RemoveXAttr(attr, (*Context)(&header.Context))
}

func (c *FileSystemConnector) SetXAttr(header *raw.InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status {
	node := c.toInode(header.NodeId)
	return node.fsInode.SetXAttr(attr, data, int(input.Flags), (*Context)(&header.Context))
}

func (c *FileSystemConnector) ListXAttr(header *raw.InHeader) (data []byte, code Status) {
	node := c.toInode(header.NodeId)
	attrs, code := node.fsInode.ListXAttr((*Context)(&header.Context))
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

func (c *FileSystemConnector) Write(header *raw.InHeader, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	node := c.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Write(data, int64(input.Offset))
}

func (c *FileSystemConnector) Read(header *raw.InHeader, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	node := c.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)

	return opened.WithFlags.File.Read(buf, int64(input.Offset))
}

func (c *FileSystemConnector) StatFs(out *StatfsOut, header *raw.InHeader) Status {
	node := c.toInode(header.NodeId)
	s := node.FsNode().StatFs()
	if s == nil {
		return ENOSYS
	}
	*out = *s
	return OK
}

func (c *FileSystemConnector) Flush(header *raw.InHeader, input *raw.FlushIn) Status {
	node := c.toInode(header.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Flush()
}
