// Translation of raw operation to path based operations.

package fuse

import (
	"bytes"
	"path/filepath"
	"time"
)


func NewFileSystemConnector(fs FileSystem, opts *MountOptions) (out *FileSystemConnector) {
	out = EmptyFileSystemConnector()
	if code := out.Mount("/", fs, opts); code != OK {
		panic("root mount failed.")
	}
	out.verify()

	return out
}

func (me *FileSystemConnector) Destroy(h *InHeader, input *InitIn) {
	// TODO - umount all.
}

func (me *FileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, status Status) {
	parent := me.getInodeData(header.NodeId)
	return me.internalLookup(parent, name, 1)
}

func (me *FileSystemConnector) internalLookup(parent *inode, name string, lookupCount int) (out *EntryOut, status Status) {
	out, status, _ = me.internalLookupWithNode(parent, name, lookupCount)
	return out, status
}

func (me *FileSystemConnector) internalLookupWithNode(parent *inode, name string, lookupCount int) (out *EntryOut, status Status, node *inode) {
	// TODO - fuse.c has special case code for name == "." and
	// "..", those lookups happen if FUSE_EXPORT_SUPPORT is set in
	// Init.
	fullPath, mount := parent.GetPath()
	if mount == nil {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}
	fullPath = filepath.Join(fullPath, name)

	attr, err := mount.fs.GetAttr(fullPath)

	if err == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}

	if err != OK {
		return nil, err, nil
	}

	data := me.lookupUpdate(parent, name, attr.Mode&S_IFDIR != 0)
	data.LookupCount += lookupCount

	out = &EntryOut{
		NodeId:     data.NodeId,
		Generation: 1, // where to get the generation?
	}
	SplitNs(mount.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(mount.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	out.Attr = *attr
	out.Attr.Ino = data.NodeId
	return out, OK, data
}

func (me *FileSystemConnector) Forget(h *InHeader, input *ForgetIn) {
	me.forgetUpdate(h.NodeId, int(input.Nlookup))
}

func (me *FileSystemConnector) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	// TODO - do something intelligent with input.Fh.
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	attr, err := mount.fs.GetAttr(fullPath)
	if err != OK {
		return nil, err
	}

	out = &AttrOut{
		Attr: *attr,
	}
	out.Attr.Ino = header.NodeId
	SplitNs(mount.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

	return out, OK
}

func (me *FileSystemConnector) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, ENOENT
	}
	// TODO - how to handle return flags, the FUSE open flags?
	stream, err := mount.fs.OpenDir(fullPath)
	if err != OK {
		return 0, 0, err
	}

	de := &Dir{
		stream: stream,
	}
	h := me.registerFile(node, de)

	return 0, h, OK
}

func (me *FileSystemConnector) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	d := me.getDir(input.Fh)
	de, code := d.ReadDir(input)
	if code != OK {
		return nil, code
	}
	return de, OK
}

func (me *FileSystemConnector) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, ENOENT
	}

	// TODO - how to handle return flags, the FUSE open flags?
	f, err := mount.fs.Open(fullPath, input.Flags)
	if err != OK {
		return 0, 0, err
	}
	h := me.registerFile(node, f)

	return 0, h, OK
}

func (me *FileSystemConnector) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	var err Status = OK

	// TODO - support Fh.   (FSetAttr/FGetAttr/FTruncate.)
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	if input.Valid&FATTR_MODE != 0 {
		err = mount.fs.Chmod(fullPath, input.Mode)
	}
	if err == OK && (input.Valid&FATTR_UID != 0 || input.Valid&FATTR_GID != 0) {
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		err = mount.fs.Chown(fullPath, uint32(input.Uid), uint32(input.Gid))
	}
	if input.Valid&FATTR_SIZE != 0 {
		mount.fs.Truncate(fullPath, input.Size)
	}
	if err == OK && (input.Valid&FATTR_ATIME != 0 || input.Valid&FATTR_MTIME != 0) {

		err = mount.fs.Utimens(fullPath,
			uint64(input.Atime*1e9)+uint64(input.Atimensec),
			uint64(input.Mtime*1e9)+uint64(input.Mtimensec))
	}
	if err == OK && (input.Valid&FATTR_ATIME_NOW != 0 || input.Valid&FATTR_MTIME_NOW != 0) {
		ns := time.Nanoseconds()
		err = mount.fs.Utimens(fullPath, uint64(ns), uint64(ns))
	}
	if err != OK {
		return nil, err
	}

	// TODO - where to get GetAttrIn.Flags / Fh ?
	return me.GetAttr(header, &GetAttrIn{})
}

func (me *FileSystemConnector) Readlink(header *InHeader) (out []byte, code Status) {
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	val, err := mount.fs.Readlink(fullPath)
	return bytes.NewBufferString(val).Bytes(), err
}

func (me *FileSystemConnector) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	fullPath = filepath.Join(fullPath, name)
	err := mount.fs.Mknod(fullPath, input.Mode, uint32(input.Rdev))
	if err != OK {
		return nil, err
	}
	return me.internalLookup(node, name, 1)
}

func (me *FileSystemConnector) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	code = mount.fs.Mkdir(filepath.Join(fullPath, name), input.Mode)
	if code == OK {
		out, code = me.internalLookup(parent, name, 1)
	}
	return out, code
}

func (me *FileSystemConnector) Unlink(header *InHeader, name string) (code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	code = mount.fs.Unlink(filepath.Join(fullPath, name))
	if code == OK {
		// Like fuse.c, we update our internal tables.
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *FileSystemConnector) Rmdir(header *InHeader, name string) (code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	code = mount.fs.Rmdir(filepath.Join(fullPath, name))
	if code == OK {
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *FileSystemConnector) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	err := mount.fs.Symlink(pointedTo, filepath.Join(fullPath, linkName))
	if err != OK {
		return nil, err
	}

	out, code = me.internalLookup(parent, linkName, 1)
	return out, code
}

func (me *FileSystemConnector) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	oldPath, oldMount, oldParent := me.GetPath(header.NodeId)
	newPath, mount, newParent := me.GetPath(input.Newdir)
	if mount == nil || oldMount == nil {
		return ENOENT
	}
	if mount != oldMount {
		return EXDEV
	}

	oldPath = filepath.Join(oldPath, oldName)
	newPath = filepath.Join(newPath, newName)
	code = mount.fs.Rename(oldPath, newPath)
	if code == OK {
		me.renameUpdate(oldParent, oldName, newParent, newName)
	}
	return code
}

func (me *FileSystemConnector) Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status) {
	orig, mount, _ := me.GetPath(input.Oldnodeid)
	newName, newMount, newParent := me.GetPath(header.NodeId)

	if mount == nil || newMount == nil {
		return nil, ENOENT
	}
	if mount != newMount {
		return nil, EXDEV
	}
	newName = filepath.Join(newName, filename)
	err := mount.fs.Link(orig, newName)

	if err != OK {
		return nil, err
	}

	return me.internalLookup(newParent, filename, 1)
}

func (me *FileSystemConnector) Access(header *InHeader, input *AccessIn) (code Status) {
	p, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	return mount.fs.Access(p, input.Mask)
}

func (me *FileSystemConnector) Create(header *InHeader, input *CreateIn, name string) (flags uint32, h uint64, out *EntryOut, code Status) {
	directory, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, nil, ENOENT
	}
	fullPath := filepath.Join(directory, name)

	f, err := mount.fs.Create(fullPath, uint32(input.Flags), input.Mode)
	if err != OK {
		return 0, 0, nil, err
	}

	out, code, inode := me.internalLookupWithNode(parent, name, 1)
	return 0, me.registerFile(inode, f), out, code
}

func (me *FileSystemConnector) Release(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	f := me.unregisterFile(node, input.Fh).(File)
	f.Release()
	me.considerDropInode(node)
}

func (me *FileSystemConnector) ReleaseDir(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	d := me.unregisterFile(node, input.Fh).(RawDir)
	d.Release()
	me.considerDropInode(node)
}

func (me *FileSystemConnector) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	// What the heck is FsyncDir supposed to do?
	return OK
}

func (me *FileSystemConnector) GetXAttr(header *InHeader, attribute string) (data []byte, code Status) {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	data, code = mount.fs.GetXAttr(path, attribute)
	return data, code
}

func (me *FileSystemConnector) RemoveXAttr(header *InHeader, attr string) Status {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}

	return mount.fs.RemoveXAttr(path, attr)
}

func (me *FileSystemConnector) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}

	return mount.fs.SetXAttr(path, attr, data, int(input.Flags))
}

func (me *FileSystemConnector) ListXAttr(header *InHeader) (data []byte, code Status) {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	attrs, code := mount.fs.ListXAttr(path)
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

func (me *FileSystemConnector) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	f := me.getFile(input.Fh).(File)
	return f.Write(input, data)
}

func (me *FileSystemConnector) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	f := me.getFile(input.Fh)
	return f.Read(input, bp)
}
