// Translation of raw operation to path based operations.

package fuse

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

var _ = fmt.Println

func NewFileSystemConnector(fs FileSystem, opts *FileSystemOptions) (out *FileSystemConnector) {
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
		timeout := me.rootNode.mount.options.NegativeTimeout
		if timeout > 0 {
			return NegativeEntry(timeout), OK, nil
		} else {
			return nil, ENOENT, nil
		}
	}
	fullPath = filepath.Join(fullPath, name)

	fi, err := mount.fs.GetAttr(fullPath)

	if err == ENOENT && mount.options.NegativeTimeout > 0.0 {
		return NegativeEntry(mount.options.NegativeTimeout), OK, nil
	}

	if err != OK {
		return nil, err, nil
	}

	data := me.lookupUpdate(parent, name, fi.IsDirectory())
	data.LookupCount += lookupCount

	out = &EntryOut{
		NodeId:     data.NodeId,
		Generation: 1, // where to get the generation?
	}
	SplitNs(mount.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(mount.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	CopyFileInfo(fi, &out.Attr)
	out.Attr.Ino = data.NodeId
	return out, OK, data
}

func (me *FileSystemConnector) Forget(h *InHeader, input *ForgetIn) {
	me.forgetUpdate(h.NodeId, int(input.Nlookup))
}

func (me *FileSystemConnector) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	if input.Flags&FUSE_GETATTR_FH != 0 {
		f, bridge := me.getFile(input.Fh)
		fi, err := f.GetAttr()
		if err != OK && err != ENOSYS {
			return nil, err
		}

		if fi != nil {
			out = &AttrOut{}
			CopyFileInfo(fi, &out.Attr)
			out.Attr.Ino = header.NodeId
			SplitNs(bridge.mountData.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

			return out, OK
		}
	}

	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	fi, err := mount.fs.GetAttr(fullPath)
	if err != OK {
		return nil, err
	}

	out = &AttrOut{}
	CopyFileInfo(fi, &out.Attr)
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
	h := me.registerFile(node, mount, de, input.Flags)

	return 0, h, OK
}

func (me *FileSystemConnector) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	d, _ := me.getDir(input.Fh)
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
	h := me.registerFile(node, mount, f, input.Flags)

	return 0, h, OK
}

func (me *FileSystemConnector) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	var err Status = OK
	var getAttrIn GetAttrIn
	fh := uint64(0)
	if input.Valid&FATTR_FH != 0 {
		fh = input.Fh
		getAttrIn.Fh = fh
		getAttrIn.Flags |= FUSE_GETATTR_FH
	}

	f, mount, fullPath := me.getOpenFileData(header.NodeId, fh)
	if mount == nil {
		return nil, ENOENT
	}

	fileResult := ENOSYS
	if err.Ok() && input.Valid&FATTR_MODE != 0 {
		permissions := uint32(07777) & input.Mode
		if f != nil {
			fileResult = f.Chmod(permissions)
		}
		if fileResult == ENOSYS {
			err = mount.fs.Chmod(fullPath, permissions)
		} else {
			err = fileResult
			fileResult = ENOSYS
		}
	}
	if err.Ok() && (input.Valid&(FATTR_UID|FATTR_GID) != 0) {
		if f != nil {
			fileResult = f.Chown(uint32(input.Uid), uint32(input.Gid))
		}

		if fileResult == ENOSYS {
			// TODO - can we get just FATTR_GID but not FATTR_UID ?
			err = mount.fs.Chown(fullPath, uint32(input.Uid), uint32(input.Gid))
		} else {
			err = fileResult
			fileResult = ENOSYS
		}
	}
	if err.Ok() && input.Valid&FATTR_SIZE != 0 {
		if f != nil {
			fileResult = f.Truncate(input.Size)
		}
		if fileResult == ENOSYS {
			err = mount.fs.Truncate(fullPath, input.Size)
		} else {
			err = fileResult
			fileResult = ENOSYS
		}
	}
	if err.Ok() && (input.Valid&(FATTR_ATIME|FATTR_MTIME|FATTR_ATIME_NOW|FATTR_MTIME_NOW) != 0) {
		atime := uint64(input.Atime*1e9) + uint64(input.Atimensec)
		if input.Valid&FATTR_ATIME_NOW != 0 {
			atime = uint64(time.Nanoseconds())
		}

		mtime := uint64(input.Mtime*1e9) + uint64(input.Mtimensec)
		if input.Valid&FATTR_MTIME_NOW != 0 {
			mtime = uint64(time.Nanoseconds())
		}

		if f != nil {
			fileResult = f.Utimens(atime, mtime)
		}
		if fileResult == ENOSYS {
			err = mount.fs.Utimens(fullPath, atime, mtime)
		} else {
			err = fileResult
			fileResult = ENOSYS
		}
	}
	if err != OK {
		return nil, err
	}

	return me.GetAttr(header, &getAttrIn)
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
	if code.Ok() {
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
	if code.Ok() {
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
	if code.Ok() {
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
	if code.Ok() {
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
	return 0, me.registerFile(inode, mount, f, input.Flags), out, code
}

func (me *FileSystemConnector) Release(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	f := me.unregisterFile(node, input.Fh).(File)
	f.Release()
}

func (me *FileSystemConnector) Flush(input *FlushIn) Status {
	f, b := me.getFile(input.Fh)

	code := f.Flush()
	if code.Ok() && b.Flags&O_ANYWRITE != 0 {
		// We only signal releases to the FS if the
		// open could have changed things.
		var path string
		var mount *mountData
		me.treeLock.RLock()
		if b.inode.Parent != nil {
			path, mount = b.inode.GetPath()
		}
		me.treeLock.RUnlock()

		if mount != nil {
			code = mount.fs.Flush(path)
		}
	}
	return code
}

func (me *FileSystemConnector) ReleaseDir(header *InHeader, input *ReleaseIn) {
	node := me.getInodeData(header.NodeId)
	d := me.unregisterFile(node, input.Fh).(rawDir)
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

func (me *FileSystemConnector) fileDebug(fh uint64, n *inode) {
	p, _, _ := me.GetPath(n.NodeId)
	log.Printf("Fh %d = %s", fh, p)
}

func (me *FileSystemConnector) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	f, b := me.getFile(input.Fh)
	if me.Debug {
		me.fileDebug(input.Fh, b.inode)
	}
	return f.Write(input, data)
}

func (me *FileSystemConnector) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	f, b := me.getFile(input.Fh)
	if me.Debug {
		me.fileDebug(input.Fh, b.inode)
	}
	return f.Read(input, bp)
}

func (me *FileSystemConnector) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, data []byte, code Status) {
	f, b := me.getFile(input.Fh)
	if me.Debug {
		me.fileDebug(input.Fh, b.inode)
	}
	return f.Ioctl(input)
}
