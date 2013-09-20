package pathfs

import (
	"syscall"

	"github.com/hanwen/go-fuse/fuse"
)

func (fs *loopbackFileSystem) StatFs(name string) *fuse.StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.GetPath(name), &s)
	if err == nil {
		return &fuse.StatfsOut{
			Blocks: s.Blocks,
			Bsize:  uint32(s.Bsize),
			Bfree:  s.Bfree,
			Bavail: s.Bavail,
			Files:  s.Files,
			Ffree:  s.Ffree,
		}
	}
	return nil
}
