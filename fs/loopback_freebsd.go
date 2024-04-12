package fs

import (
	"context"
	"syscall"
)

var _ = (NodeGetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

var _ = (NodeSetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	return syscall.ENOSYS
}

var _ = (NodeRemovexattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
	return syscall.ENOSYS
}

var _ = (NodeListxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

var _ = (NodeCopyFileRanger)((*LoopbackNode)(nil))

func (n *LoopbackNode) CopyFileRange(ctx context.Context, fhIn FileHandle,
	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
	len uint64, flags uint64) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}

func intDev(dev uint32) uint64 {
	return uint64(dev)
}
