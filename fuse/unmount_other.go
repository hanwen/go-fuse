//go:build !linux

package fuse

import "syscall"

// detachUnmount falls back to a plain unmount on non-linux systems that
// do not have MNT_DETACH. FUSE-pinning scenarios that motivated Shutdown
// are linux-specific (overlayfs), so the fallback is sufficient.
func detachUnmount(mountPoint string) error {
	return syscall.Unmount(mountPoint, 0)
}
