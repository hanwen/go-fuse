package openat

import (
	"golang.org/x/sys/unix"
)

const (
	searchFlags = unix.O_PATH
	symlinkFlag = 0
)

func openatNoSymlinks(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	how := unix.OpenHow{
		// os/exec expects all fds to have O_CLOEXEC or it will leak them to subprocesses.
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	}
	fd, err = unix.Openat2(dirfd, path, &how)
	if err == unix.ENOSYS {
		return walk(dirfd, path, flags, mode)
	}
	return fd, err
}
