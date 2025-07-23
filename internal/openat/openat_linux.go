package openat

import (
	"golang.org/x/sys/unix"
)

func openatNoSymlinks(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	how := unix.OpenHow{
		// os/exec expects all fds to have O_CLOEXEC or it will leak them to subprocesses.
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	}
	fd, err = unix.Openat2(dirfd, path, &how)
	if err != nil && err == unix.ENOSYS {
		flags |= unix.O_NOFOLLOW | unix.O_CLOEXEC
		fd, err = unix.Openat(dirfd, path, flags, mode)
	}
	return fd, err
}
