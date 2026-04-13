package fuse

import "syscall"

// detachUnmount issues a lazy (MS_DETACH) unmount: the kernel drops the
// mount from the current namespace immediately and cleans up when the
// last reference drops. Used by Server.Shutdown to guarantee forward
// progress even when the FUSE superblock is pinned by an external
// reference (e.g. an overlayfs lower layer).
func detachUnmount(mountPoint string) error {
	return syscall.Unmount(mountPoint, syscall.MNT_DETACH)
}
