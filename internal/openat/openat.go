package openat

// OpenatNofollow is a symlink-safe syscall.Openat replacement.
//
// On Linux, it calls openat2(2) with RESOLVE_NO_SYMLINKS. This prevents following
// symlinks in any component of the path.
//
// On other platforms, it calls openat(2) with O_NOFOLLOW.
// TODO: This is insecure as O_NOFOLLOW only affects the final path component.
func OpenatNofollow(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	return openatNofollow(dirfd, path, flags, mode)
}
