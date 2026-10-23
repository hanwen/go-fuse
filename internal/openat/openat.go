package openat

import (
	"strings"

	"golang.org/x/sys/unix"
)

// Openat opens path relative to dirfd without following symlinks in
// any component. The final component is opened with O_NOFOLLOW,
// except on Darwin when flags has O_SYMLINK. path must be relative
// and must not contain "..".
func Openat(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	if path == "" || path[0] == '/' {
		return -1, unix.EINVAL
	}
	for _, c := range strings.Split(path, "/") {
		if c == ".." {
			return -1, unix.EINVAL
		}
	}
	return openatNoSymlinks(dirfd, path, flags, mode)
}

func walk(dirfd int, path string, flags int, mode uint32) (int, error) {
	comps := strings.Split(path, "/")
	cur := dirfd
	for _, c := range comps[:len(comps)-1] {
		if c == "" || c == "." {
			continue
		}
		fd, err := unix.Openat(cur, c, searchFlags|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if cur != dirfd {
			unix.Close(cur)
		}
		if err != nil {
			return -1, err
		}
		cur = fd
	}
	if cur != dirfd {
		defer unix.Close(cur)
	}
	if flags&symlinkFlag == 0 {
		flags |= unix.O_NOFOLLOW
	}
	// os/exec expects all fds to have O_CLOEXEC or it will leak them to subprocesses.
	return unix.Openat(cur, comps[len(comps)-1], flags|unix.O_CLOEXEC, mode)
}
