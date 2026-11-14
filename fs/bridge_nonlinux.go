//go:build !linux

package fs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v3/fuse"
)

func (b *rawBridge) Statx(ctx context.Context, in *fuse.StatxIn, out *fuse.StatxOut) syscall.Errno {
	return fuse.ENOSYS
}
