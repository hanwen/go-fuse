//go:build !linux

package openat

func openatNoSymlinks(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	return walk(dirfd, path, flags, mode)
}
