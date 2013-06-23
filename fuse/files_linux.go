package fuse

import (
	"time"
	"syscall"
)

func (f *loopbackFile) Allocate(off uint64, sz uint64, mode uint32) Status {
	f.lock.Lock()
	err := syscall.Fallocate(int(f.File.Fd()), mode, int64(off), int64(sz))
	f.lock.Unlock()
	if err != nil {
		return ToStatus(err)
	}
	return OK
}

const _UTIME_NOW = ((1 << 30) - 1)
const _UTIME_OMIT = ((1 << 30) - 2)

func (f *loopbackFile) Utimens(a *time.Time, m *time.Time) Status {
	tv := make([]syscall.Timeval, 2)
	if a == nil {
		tv[0].Usec = _UTIME_OMIT
	} else {
		n := a.UnixNano()
		tv[0] = syscall.NsecToTimeval(n)
 	}

	if m == nil {
		tv[1].Usec = _UTIME_OMIT
	} else {
		n := a.UnixNano()
		tv[1] = syscall.NsecToTimeval(n)
 	}
	
	err := syscall.Futimes(int(f.File.Fd()), tv)
	return ToStatus(err)	
}
