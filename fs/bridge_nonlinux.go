//go:build !linux

package fs

import (
	"context"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func (b *rawBridge) Statx(ctx context.Context, in *fuse.StatxIn, out *fuse.StatxOut) fuse.Status {
	return fuse.ENOSYS
}
