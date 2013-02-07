package fuse

import (
	"syscall"
)

func (fs *LoopbackFileSystem) StatFs(name string) *StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.GetPath(name), &s)
	if err == nil {
		return &StatfsOut{
			Blocks:  s.Blocks,
			Bsize:   uint32(s.Bsize),
			Bfree:   s.Bfree,
			Bavail:  s.Bavail,
			Files:   s.Files,
			Ffree:   s.Ffree,
			Frsize:  uint32(s.Frsize),
			NameLen: uint32(s.Namelen),
		}
	}
	return nil
}

func (fs *LoopbackFileSystem) ListXAttr(name string, context *Context) ([]string, Status) {
	data, errNo := ListXAttr(fs.GetPath(name))

	return data, Status(errNo)
}

func (fs *LoopbackFileSystem) RemoveXAttr(name string, attr string, context *Context) Status {
	return Status(Removexattr(fs.GetPath(name), attr))
}

func (fs *LoopbackFileSystem) String() string {
	return fmt.Sprintf("LoopbackFs(%s)", fs.Root)
}

func (fs *LoopbackFileSystem) GetXAttr(name string, attr string, context *Context) ([]byte, Status) {
	data := make([]byte, 1024)
	data, errNo := GetXAttr(fs.GetPath(name), attr, data)

	return data, Status(errNo)
}
