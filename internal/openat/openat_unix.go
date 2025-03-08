//go:build !linux

package openat

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// TODO: This is insecure as O_NOFOLLOW only affects the final path component.
// See https://github.com/rfjakob/gocryptfs/issues/165 for how this could be handled.
func openatNofollow(dirfd int, path string, flags int, mode uint32) (fd int, err error) {
	// os/exec expects all fds to have O_CLOEXEC or it will leak fds to subprocesses.
	flags |= syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	return unix.Openat(dirfd, path, flags, mode)
}
