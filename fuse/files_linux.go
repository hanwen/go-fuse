package fuse

import (
	"syscall"
)

func (f *LoopbackFile) Allocate(off uint64, sz uint64, mode uint32) Status {
	f.lock.Lock()
	err := syscall.Fallocate(int(f.File.Fd()), mode, int64(off), int64(sz))
	f.lock.Unlock()
	if err != nil {
		return ToStatus(err)
	}
	return OK
}
