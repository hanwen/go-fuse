package pathfs

import (
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

func (fs *loopbackFileSystem) StatFs(name string) *fuse.StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.GetPath(name), &s)
	if err == nil {
		return &fuse.StatfsOut{
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

func (fs *loopbackFileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	data, errNo := ListXAttr(fs.GetPath(name))

	return data, fuse.Status(errNo)
}

func (fs *loopbackFileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	return fuse.Status(Removexattr(fs.GetPath(name), attr))
}

func (fs *loopbackFileSystem) String() string {
	return fmt.Sprintf("LoopbackFs(%s)", fs.Root)
}

func (fs *loopbackFileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	data := make([]byte, 1024)
	data, errNo := GetXAttr(fs.GetPath(name), attr, data)

	return data, fuse.Status(errNo)
}
