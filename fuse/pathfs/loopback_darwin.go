package fuse

import (
	"syscall"
)

func (fs *LoopbackFileSystem) StatFs(name string) *StatfsOut {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.GetPath(name), &s)
	if err == nil {
		return &StatfsOut{
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
