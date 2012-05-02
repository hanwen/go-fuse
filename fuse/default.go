package fuse

import ()

// DefaultFileSystem
func (fs *DefaultFileSystem) GetAttr(name string, context *Context) (*Attr, Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status {
	return ENOSYS
}

func (fs *DefaultFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return ENOSYS
}

func (fs *DefaultFileSystem) Readlink(name string, context *Context) (string, Status) {
	return "", ENOSYS
}

func (fs *DefaultFileSystem) Mknod(name string, mode uint32, dev uint32, context *Context) Status {
	return ENOSYS
}

func (fs *DefaultFileSystem) Mkdir(name string, mode uint32, context *Context) Status {
	return ENOSYS
}

func (fs *DefaultFileSystem) Unlink(name string, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Rmdir(name string, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Symlink(value string, linkName string, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Rename(oldName string, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Link(oldName string, newName string, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Chmod(name string, mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Chown(name string, uid uint32, gid uint32, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Truncate(name string, offset uint64, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Open(name string, flags uint32, context *Context) (file File, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) OpenDir(name string, context *Context) (stream chan DirEntry, status Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) OnMount(nodeFs *PathNodeFs) {
}

func (fs *DefaultFileSystem) OnUnmount() {
}

func (fs *DefaultFileSystem) Access(name string, mode uint32, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status) {
	return nil, ENOSYS
}

func (fs *DefaultFileSystem) Utimens(name string, AtimeNs int64, CtimeNs int64, context *Context) (code Status) {
	return ENOSYS
}

func (fs *DefaultFileSystem) String() string {
	return "DefaultFileSystem"
}

func (fs *DefaultFileSystem) StatFs(name string) *StatfsOut {
	return nil
}
