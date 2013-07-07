package nodefs

// This file contains FileSystemConnector's implementation of
// RawFileSystem

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Println

// Returns the RawFileSystem so it can be mounted.
func (c *FileSystemConnector) RawFS() fuse.RawFileSystem {
	return (*rawBridge)(c)
}

type rawBridge FileSystemConnector

func (c *rawBridge) Fsync(input *raw.FsyncIn) fuse.Status {
	return fuse.ENOSYS
}

func (c *rawBridge) SetDebug(debug bool) {
	c.fsConn().SetDebug(debug)
}

func (c *rawBridge) FsyncDir(input *raw.FsyncIn) fuse.Status {
	return fuse.ENOSYS
}

func (c *rawBridge) fsConn() *FileSystemConnector {
	return (*FileSystemConnector)(c)
}

func (c *rawBridge) String() string {
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

func (c *rawBridge) Init(s *fuse.Server) {
	c.server = s
}

func (c *FileSystemConnector) lookupMountUpdate(out *fuse.Attr, mount *fileSystemMount) (node *Inode, code fuse.Status) {
	code = mount.fs.Root().GetAttr(out, nil, nil)
	if !code.Ok() {
		log.Println("Root getattr should not return error", code)
		out.Mode = fuse.S_IFDIR | 0755
		return mount.mountInode, fuse.OK
	}

	return mount.mountInode, fuse.OK
}

func (c *FileSystemConnector) internalLookup(out *fuse.Attr, parent *Inode, name string, header *raw.InHeader) (node *Inode, code fuse.Status) {
	child := parent.GetChild(name)
	if child != nil && child.mountPoint != nil {
		return c.lookupMountUpdate(out, child.mountPoint)
	}

	if child != nil {
		parent = nil
	}
	var fsNode Node
	if child != nil {
		code = child.fsInode.GetAttr(out, nil, context(header))
		fsNode = child.Node()
	} else {
		fsNode, code = parent.fsInode.Lookup(out, name, context(header))
	}

	if child == nil && fsNode != nil {
		child = fsNode.Inode()
		if child == nil {
			log.Panicf("Lookup %q returned child without Inode: %v", name, fsNode)
		}
	}

	return child, code
}

func (c *rawBridge) Lookup(header *raw.InHeader, name string, out *raw.EntryOut) (code fuse.Status) {
	parent := c.toInode(header.NodeId)
	if !parent.IsDir() {
		log.Printf("Lookup %q called on non-Directory node %d", name, header.NodeId)
		return fuse.ENOTDIR
	}
	outAttr := (*fuse.Attr)(&out.Attr)
	child, code := c.fsConn().internalLookup(outAttr, parent, name, header)
	if code == fuse.ENOENT && parent.mount.negativeEntry(out) {
		return fuse.OK
	}
	if !code.Ok() {
		return code
	}
	if child == nil {
		log.Println("Lookup returned fuse.OK with nil child", name)
	}

	child.mount.fillEntry(out)
	out.NodeId = c.fsConn().lookupUpdate(child)
	out.Generation = child.generation
	out.Ino = out.NodeId

	return fuse.OK
}

func (c *rawBridge) Forget(nodeID, nlookup uint64) {
	c.fsConn().forgetUpdate(nodeID, int(nlookup))
}

func (c *rawBridge) GetAttr(input *raw.GetAttrIn, out *raw.AttrOut) (code fuse.Status) {
	node := c.toInode(input.NodeId)

	var f File
	if input.Flags()&raw.FUSE_GETATTR_FH != 0 {
		if opened := node.mount.getOpenedFile(input.Fh()); opened != nil {
			f = opened.WithFlags.File
		}
	}

	dest := (*fuse.Attr)(&out.Attr)
	code = node.fsInode.GetAttr(dest, f, context(&input.InHeader))
	if !code.Ok() {
		return code
	}

	node.mount.fillAttr(out, input.NodeId)
	return fuse.OK
}

func (c *rawBridge) OpenDir(input *raw.OpenIn, out *raw.OpenOut) (code fuse.Status) {
	node := c.toInode(input.NodeId)
	stream, err := node.fsInode.OpenDir(context(&input.InHeader))
	if err != fuse.OK {
		return err
	}
	stream = append(stream, node.getMountDirEntries()...)
	de := &connectorDir{
		node: node.Node(),
		stream: append(stream,
			fuse.DirEntry{fuse.S_IFDIR, "."},
			fuse.DirEntry{fuse.S_IFDIR, ".."}),
		rawFS: c,
	}
	h, opened := node.mount.registerFileHandle(node, de, nil, input.Flags)
	out.OpenFlags = opened.FuseFlags
	out.Fh = h
	return fuse.OK
}

func (c *rawBridge) ReadDir(input *raw.ReadIn, out *fuse.DirEntryList) fuse.Status {
	node := c.toInode(input.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.dir.ReadDir(input, out)
}

func (c *rawBridge) ReadDirPlus(input *raw.ReadIn, out *fuse.DirEntryList) fuse.Status {
	node := c.toInode(input.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.dir.ReadDirPlus(input, out)
}

func context(header *raw.InHeader) *fuse.Context {
	return (*fuse.Context)(&header.Context)
}

func (c *rawBridge) Open(input *raw.OpenIn, out *raw.OpenOut) (status fuse.Status) {
	node := c.toInode(input.NodeId)
	f, code := node.fsInode.Open(input.Flags, context(&input.InHeader))
	if !code.Ok() {
		return code
	}
	h, opened := node.mount.registerFileHandle(node, nil, f, input.Flags)
	out.OpenFlags = opened.FuseFlags
	out.Fh = h
	return fuse.OK
}

func (c *rawBridge) SetAttr(input *raw.SetAttrIn, out *raw.AttrOut) (code fuse.Status) {
	node := c.toInode(input.NodeId)

	ctx := context(&input.InHeader)

	var f File
	if input.Valid&raw.FATTR_FH != 0 {
		opened := node.mount.getOpenedFile(input.Fh)
		f = opened.WithFlags.File
	}

	if code.Ok() && input.Valid&raw.FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		code = node.fsInode.Chmod(f, permissions, ctx)
	}
	if code.Ok() && (input.Valid&(raw.FATTR_UID|raw.FATTR_GID) != 0) {
		code = node.fsInode.Chown(f, uint32(input.Uid), uint32(input.Gid), ctx)
	}
	if code.Ok() && input.Valid&raw.FATTR_SIZE != 0 {
		code = node.fsInode.Truncate(f, input.Size, ctx)
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

		code = node.fsInode.Utimens(f, atime, mtime, ctx)
	}

	if !code.Ok() {
		return code
	}

	// Must call GetAttr(); the filesystem may override some of
	// the changes we effect here.
	attr := (*fuse.Attr)(&out.Attr)
	code = node.fsInode.GetAttr(attr, nil, ctx)
	if code.Ok() {
		node.mount.fillAttr(out, input.NodeId)
	}
	return code
}

func (c *rawBridge) Fallocate(in *raw.FallocateIn) (code fuse.Status) {
	n := c.toInode(in.NodeId)
	opened := n.mount.getOpenedFile(in.Fh)

	return n.fsInode.Fallocate(opened, in.Offset, in.Length, in.Mode, context(&in.InHeader))
}

func (c *rawBridge) Readlink(header *raw.InHeader) (out []byte, code fuse.Status) {
	n := c.toInode(header.NodeId)
	return n.fsInode.Readlink(context(header))
}

func (c *rawBridge) Mknod(input *raw.MknodIn, name string, out *raw.EntryOut) (code fuse.Status) {
	parent := c.toInode(input.NodeId)
	ctx := context(&input.InHeader)
	fsNode, code := parent.fsInode.Mknod(name, input.Mode, uint32(input.Rdev), ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*fuse.Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *rawBridge) Mkdir(input *raw.MkdirIn, name string, out *raw.EntryOut) (code fuse.Status) {
	parent := c.toInode(input.NodeId)
	ctx := context(&input.InHeader)
	fsNode, code := parent.fsInode.Mkdir(name, input.Mode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*fuse.Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *rawBridge) Unlink(header *raw.InHeader, name string) (code fuse.Status) {
	parent := c.toInode(header.NodeId)
	return parent.fsInode.Unlink(name, context(header))
}

func (c *rawBridge) Rmdir(header *raw.InHeader, name string) (code fuse.Status) {
	parent := c.toInode(header.NodeId)
	return parent.fsInode.Rmdir(name, context(header))
}

func (c *rawBridge) Symlink(header *raw.InHeader, pointedTo string, linkName string, out *raw.EntryOut) (code fuse.Status) {
	parent := c.toInode(header.NodeId)
	ctx := context(header)
	fsNode, code := parent.fsInode.Symlink(linkName, pointedTo, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*fuse.Attr)(&out.Attr), nil, ctx)
	}
	return code
}

func (c *rawBridge) Rename(input *raw.RenameIn, oldName string, newName string) (code fuse.Status) {
	oldParent := c.toInode(input.NodeId)

	child := oldParent.GetChild(oldName)
	if child.mountPoint != nil {
		return fuse.EBUSY
	}

	newParent := c.toInode(input.Newdir)
	if oldParent.mount != newParent.mount {
		return fuse.EXDEV
	}

	return oldParent.fsInode.Rename(oldName, newParent.fsInode, newName, context(&input.InHeader))
}

func (c *rawBridge) Link(input *raw.LinkIn, name string, out *raw.EntryOut) (code fuse.Status) {
	existing := c.toInode(input.Oldnodeid)
	parent := c.toInode(input.NodeId)

	if existing.mount != parent.mount {
		return fuse.EXDEV
	}
	ctx := context(&input.InHeader)
	fsNode, code := parent.fsInode.Link(name, existing.fsInode, ctx)
	if code.Ok() {
		c.childLookup(out, fsNode)
		code = fsNode.GetAttr((*fuse.Attr)(&out.Attr), nil, ctx)
	}

	return code
}

func (c *rawBridge) Access(input *raw.AccessIn) (code fuse.Status) {
	n := c.toInode(input.NodeId)
	return n.fsInode.Access(input.Mask, context(&input.InHeader))
}

func (c *rawBridge) Create(input *raw.CreateIn, name string, out *raw.CreateOut) (code fuse.Status) {
	parent := c.toInode(input.NodeId)
	f, fsNode, code := parent.fsInode.Create(name, uint32(input.Flags), input.Mode, context(&input.InHeader))
	if !code.Ok() {
		return code
	}

	c.childLookup(&out.EntryOut, fsNode)
	handle, opened := parent.mount.registerFileHandle(fsNode.Inode(), nil, f, input.Flags)

	out.OpenOut.OpenFlags = opened.FuseFlags
	out.OpenOut.Fh = handle
	return code
}

func (c *rawBridge) Release(input *raw.ReleaseIn) {
	node := c.toInode(input.NodeId)
	opened := node.mount.unregisterFileHandle(input.Fh, node)
	opened.WithFlags.File.Release()
}

func (c *rawBridge) ReleaseDir(input *raw.ReleaseIn) {
	node := c.toInode(input.NodeId)
	node.mount.unregisterFileHandle(input.Fh, node)
}

func (c *rawBridge) GetXAttrSize(header *raw.InHeader, attribute string) (sz int, code fuse.Status) {
	node := c.toInode(header.NodeId)
	data, errno := node.fsInode.GetXAttr(attribute, context(header))
	return len(data), errno
}

func (c *rawBridge) GetXAttrData(header *raw.InHeader, attribute string) (data []byte, code fuse.Status) {
	node := c.toInode(header.NodeId)
	return node.fsInode.GetXAttr(attribute, context(header))
}

func (c *rawBridge) RemoveXAttr(header *raw.InHeader, attr string) fuse.Status {
	node := c.toInode(header.NodeId)
	return node.fsInode.RemoveXAttr(attr, context(header))
}

func (c *rawBridge) SetXAttr(input *raw.SetXAttrIn, attr string, data []byte) fuse.Status {
	node := c.toInode(input.NodeId)
	return node.fsInode.SetXAttr(attr, data, int(input.Flags), context(&input.InHeader))
}

func (c *rawBridge) ListXAttr(header *raw.InHeader) (data []byte, code fuse.Status) {
	node := c.toInode(header.NodeId)
	attrs, code := node.fsInode.ListXAttr(context(header))
	if code != fuse.OK {
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

func (c *rawBridge) Write(input *raw.WriteIn, data []byte) (written uint32, code fuse.Status) {
	node := c.toInode(input.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Write(data, int64(input.Offset))
}

func (c *rawBridge) Read(input *raw.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	node := c.toInode(input.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)

	return opened.WithFlags.File.Read(buf, int64(input.Offset))
}

func (c *rawBridge) StatFs(header *raw.InHeader, out *raw.StatfsOut) fuse.Status {
	node := c.toInode(header.NodeId)
	s := node.Node().StatFs()
	if s == nil {
		return fuse.ENOSYS
	}
	*out = *(*raw.StatfsOut)(s)
	return fuse.OK
}

func (c *rawBridge) Flush(input *raw.FlushIn) fuse.Status {
	node := c.toInode(input.NodeId)
	opened := node.mount.getOpenedFile(input.Fh)
	return opened.WithFlags.File.Flush()
}
