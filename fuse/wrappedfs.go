package fuse

type WrappingPathFilesystem struct {
	original PathFilesystem
}

func (me *WrappingPathFilesystem) GetAttr(name string) (*Attr, Status) {
	return me.original.GetAttr(name)
}

func (me *WrappingPathFilesystem) Readlink(name string) (string, Status) {
	return me.original.Readlink(name)
}

func (me *WrappingPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	return me.original.Mknod(name, mode, dev)
}

func (me *WrappingPathFilesystem) Mkdir(name string, mode uint32) Status {
	return me.original.Mkdir(name, mode)
}

func (me *WrappingPathFilesystem) Unlink(name string) (code Status) {
	return me.original.Unlink(name)
}

func (me *WrappingPathFilesystem) Rmdir(name string) (code Status) {
	return me.original.Rmdir(name)
}

func (me *WrappingPathFilesystem) Symlink(value string, linkName string) (code Status) {
	return me.original.Symlink(value, linkName)
}

func (me *WrappingPathFilesystem) Rename(oldName string, newName string) (code Status) {
	return me.original.Rename(oldName, newName)
}

func (me *WrappingPathFilesystem) Link(oldName string, newName string) (code Status) {
	return me.original.Link(oldName, newName)
}

func (me *WrappingPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	return me.original.Chmod(name, mode)
}

func (me *WrappingPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	return me.original.Chown(name, uid, gid)
}

func (me *WrappingPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	return me.original.Truncate(name, offset)
}

func (me *WrappingPathFilesystem) Open(name string, flags uint32) (file RawFuseFile, code Status) {
	return me.original.Open(name, flags)
}

func (me *WrappingPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	return me.original.OpenDir(name)
}

func (me *WrappingPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	return me.original.Mount(conn)
}

func (me *WrappingPathFilesystem) Unmount() {
	me.original.Unmount()
}

func (me *WrappingPathFilesystem) Access(name string, mode uint32) (code Status) {
	return me.original.Access(name, mode)
}

func (me *WrappingPathFilesystem) Create(name string, flags uint32, mode uint32) (file RawFuseFile, code Status) {
	return me.original.Create(name, flags, mode)
}

func (me *WrappingPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	return me.original.Utimens(name, AtimeNs, CtimeNs)
}


