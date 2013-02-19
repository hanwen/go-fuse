package fuse

// This file contains FileSystemConnector's implementation of
// RawFileSystem

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

func (c *FileSystemConnector) String() string {
	if c.rootNode == nil || c.rootNode.mount == nil {
		return "go-fuse:unmounted"
	}

	fs := c.rootNode.mount.fs
	name := fs.String()
	if name == "DefaultNodeFileSystem" {
		name = fmt.Sprintf("%T", fs)
		name = strings.TrimLeft(name, "*")
	}
	return name
}

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
	child := parent.GetChild(name)
	if child != nil && child.mountPoint != nil {
		return c.lookupMountUpdate(out, child.mountPoint)
	}

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

func (c *FileSystemConnector) Lookup(out *raw.EntryOut, context *Context, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	if !parent.IsDir() {
		log.Printf("Lookup %q called on non-Directory node %d", name, context.NodeId)
		return ENOTDIR
	}
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
	out.Generation = child.generation
	out.Ino = out.NodeId

	return OK
}

func (c *FileSystemConnector) Forget(nodeID, nlookup uint64) {
	c.forgetUpdate(nodeID, int(nlookup))
}

func (c *FileSystemConnector) GetAttr(out *raw.AttrOut, context *Context, input *raw.GetAttrIn) (code Status) {
	node := c.toInode(context.NodeId)

	var f File
	if input.Flags() & raw.FUSE_GETATTR_FH != 0 {
		if opened := node.mount.getOpenedFile(input.Fh()); opened != nil {
			f = opened.WithFlags.File
		}
	}

	dest := (*Attr)(&out.Attr)
	code = node.fsInode.GetAttr(dest, f, context)
	if !code.Ok() {
		return code
	}

	node.mount.fillAttr(out, context.NodeId)
	return OK
}

func (c *FileSystemConnector) OpenDir(out *raw.OpenOut, context *Context, input *raw.OpenIn) (code Status) {
	node := c.toInode(context.NodeId)
	stream, err := node.fsInode.OpenDir(context)
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

func (c *FileSystemConnector) ReadDir(l *DirEntryList, context *Context, input *raw.ReadIn) Status {
	node := c.toInode(context.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.dir.ReadDir(l, input)
}

func (c *FileSystemConnector) Open(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status) {
	node := c.toInode(context.NodeId)
	f, code := node.fsInode.Open(input.Flags, context)
	if !code.Ok() {
		return code
	}
	h, opened := node.mount.registerFileHandle(node, nil, f, input.Flags)
	out.OpenFlags = opened.FuseFlags
	out.Fh = h
	return OK
}

func (c *FileSystemConnector) SetAttr(out *raw.AttrOut, context *Context, input *raw.SetAttrIn) (code Status) {
	node := c.toInode(context.NodeId)
	var f File
	if input.Valid&raw.FATTR_FH != 0 {
		opened := node.mount.getOpenedFile(input.Fh)
		f = opened.WithFlags.File
	}

	if code.Ok() && input.Valid&raw.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, context)
	}
	if code.Ok() && (input.Valid&(raw.FATTR_UID|raw.FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), context)
	}
	if code.Ok() && input.Valid&raw.FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, context)
	}
	if code.Ok() && (input.Valid&(raw.FATTR_ATIME|raw.FATTR_MTIME|raw.FATTR_ATIME_NOW|raw.FATTR_MTIME_NOW) != 0) {
		now := time.Now()
		var atime *time.Time
		var mtime *time.Time

		if input.Valid&raw.FATTR_ATIME != 0 {
			if input.Valid&raw.FATTR_ATIME_NOW != 0 {
				atime = &now
			} else {
				t := time.Unix(int64(input.Atime), int64(input.Atimensec))
				atime = &t
			}
		}

		if input.Valid&raw.FATTR_MTIME != 0 {
			if input.Valid&raw.FATTR_MTIME_NOW != 0 {
				mtime = &now
			} else {
				t := time.Unix(int64(input.Mtime), int64(input.Mtimensec))
				mtime = &t
			}
		}

		code = node.fsInode.Utimens(f, atime, mtime, context)
	}

	if !code.Ok() {
		return code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	attr := (*Attr)(&out.Attr)
	code = node.fsInode.GetAttr(attr, nil, context)
	if code.Ok() {
		node.mount.fillAttr(out, context.NodeId)
	}
	return code
}

func (c *FileSystemConnector) Readlink(context *Context) (out []byte, code Status) {
	n := c.toInode(context.NodeId)
	return n.fsInode.Readlink(context)
}

func (c *FileSystemConnector) Mknod(out *raw.EntryOut, context *Context, input *raw.MknodIn, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	ctx := context
	fsNode, code := parent.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Mkdir(out *raw.EntryOut, context *Context, input *raw.MkdirIn, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	ctx := context
	fsNode, code := parent.fsInode.Mkdir(name, input.Mode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Unlink(context *Context, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	return parent.fsInode.Unlink(name, context)
}

func (c *FileSystemConnector) Rmdir(context *Context, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	return parent.fsInode.Rmdir(name, context)
}

func (c *FileSystemConnector) Symlink(out *raw.EntryOut, context *Context, pointedTo string, linkName string) (code Status) {
	parent := c.toInode(context.NodeId)
	ctx := context
	fsNode, code := parent.fsInode.Symlink(linkName, pointedTo, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *FileSystemConnector) Rename(context *Context, input *raw.RenameIn, oldName string, newName string) (code Status) {
	oldParent := c.toInode(context.NodeId)

	child := oldParent.GetChild(oldName)
	if child.mountPoint != nil {
		return EBUSY
	}

	newParent := c.toInode(input.Newdir)
	if oldParent.mount != newParent.mount {
		return EXDEV
	}

	return oldParent.fsInode.Rename(oldName, newParent.fsInode, newName, context)
}

func (c *FileSystemConnector) Link(out *raw.EntryOut, context *Context, input *raw.LinkIn, name string) (code Status) {
	existing := c.toInode(input.Oldnodeid)
	parent := c.toInode(context.NodeId)

	if existing.mount != parent.mount {
		return EXDEV
	}
	ctx := context
	fsNode, code := parent.fsInode.Link(name, existing.fsInode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*Attr)(&out.Attr), nil, ctx)
	}

	return code
}

func (c *FileSystemConnector) Access(context *Context, input *raw.AccessIn) (code Status) {
	n := c.toInode(context.NodeId)
	return n.fsInode.Access(input.Mask, context)
}

func (c *FileSystemConnector) Create(out *raw.CreateOut, context *Context, input *raw.CreateIn, name string) (code Status) {
	parent := c.toInode(context.NodeId)
	f, fsNode, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, context)
	if !code.Ok() {
		return code
	}

	c.childLookup(&out.EntryOut, fsNode)
	handle, opened := parent.mount.registerFileHandle(fsNode.Inode(), nil, f, input.Flags)

	out.OpenOut.OpenFlags = opened.FuseFlags
	out.OpenOut.Fh = handle
	return code
}

func (c *FileSystemConnector) Release(context *Context, input *raw.ReleaseIn) {
	node := c.toInode(context.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.WithFlags.File.Release()
}

func (c *FileSystemConnector) ReleaseDir(context *Context, input *raw.ReleaseIn) {
	node := c.toInode(context.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.dir.Release()
}

func (c *FileSystemConnector) GetXAttrSize(context *Context, attribute string) (sz int, code Status) {
	node := c.toInode(context.NodeId)
	data, errno := node.fsInode.GetXAttr(attribute, context)
	return len(data), errno
}

func (c *FileSystemConnector) GetXAttrData(context *Context, attribute string) (data []byte, code Status) {
	node := c.toInode(context.NodeId)
	return node.fsInode.GetXAttr(attribute, context)
}

func (c *FileSystemConnector) RemoveXAttr(context *Context, attr string) Status {
	node := c.toInode(context.NodeId)
	return node.fsInode.RemoveXAttr(attr, context)
}

func (c *FileSystemConnector) SetXAttr(context *Context, input *raw.SetXAttrIn, attr string, data []byte) Status {
	node := c.toInode(context.NodeId)
	return node.fsInode.SetXAttr(attr, data, int(input.Flags), context)
}

func (c *FileSystemConnector) ListXAttr(context *Context) (data []byte, code Status) {
	node := c.toInode(context.NodeId)
	attrs, code := node.fsInode.ListXAttr(context)
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

func (c *FileSystemConnector) Write(context *Context, input *raw.WriteIn, data []byte) (written uint32, code Status) {
	node := c.toInode(context.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Write(data, int64(input.Offset))
}

func (c *FileSystemConnector) Read(context *Context, input *raw.ReadIn, buf []byte) (ReadResult, Status) {
	node := c.toInode(context.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)

	return opened.WithFlags.File.Read(buf, int64(input.Offset))
}

func (c *FileSystemConnector) StatFs(out *StatfsOut, context *Context) Status {
	node := c.toInode(context.NodeId)
	s := node.FsNode().StatFs()
	if s == nil {
		return ENOSYS
	}
	*out = *s
	return OK
}

func (c *FileSystemConnector) Flush(context *Context, input *raw.FlushIn) Status {
	node := c.toInode(context.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Flush()
}
