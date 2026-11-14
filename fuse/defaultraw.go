// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"context"
	"os"
	"syscall"
)

// NewDefaultRawFileSystem returns ENOSYS (not implemented) for all
// operations.
func NewDefaultRawFileSystem() RawFileSystem {
	return (*defaultRawFileSystem)(nil)
}

type defaultRawFileSystem struct{}

func (fs *defaultRawFileSystem) Init(*InitIn, *InitOut) {
}

func (fs *defaultRawFileSystem) OnConnect(*Conn) {
}

func (fs *defaultRawFileSystem) OnUnmount() {

}

func (fs *defaultRawFileSystem) String() string {
	return os.Args[0]
}

func (fs *defaultRawFileSystem) StatFs(ctx context.Context, header *InHeader, out *StatfsOut) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Lookup(ctx context.Context, header *InHeader, name string, out *EntryOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Forget(nodeID, nlookup uint64) {
}

func (fs *defaultRawFileSystem) GetAttr(ctx context.Context, input *GetAttrIn, out *AttrOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Open(ctx context.Context, input *OpenIn, out *OpenOut) (status syscall.Errno) {
	return OK
}

func (fs *defaultRawFileSystem) SetAttr(ctx context.Context, input *SetAttrIn, out *AttrOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Readlink(ctx context.Context, header *InHeader) (out []byte, code syscall.Errno) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) Mknod(ctx context.Context, input *MknodIn, name string, out *EntryOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Mkdir(ctx context.Context, input *MkdirIn, name string, out *EntryOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Unlink(ctx context.Context, header *InHeader, name string) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rmdir(ctx context.Context, header *InHeader, name string) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Symlink(ctx context.Context, header *InHeader, pointedTo string, linkName string, out *EntryOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Rename(ctx context.Context, input *RenameIn, oldName string, newName string) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Link(ctx context.Context, input *LinkIn, name string, out *EntryOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) GetXAttr(ctx context.Context, header *InHeader, attr string, dest []byte) (size uint32, code syscall.Errno) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) SetXAttr(ctx context.Context, input *SetXAttrIn, attr string, data []byte) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ListXAttr(ctx context.Context, header *InHeader, dest []byte) (n uint32, code syscall.Errno) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) RemoveXAttr(ctx context.Context, header *InHeader, attr string) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Access(ctx context.Context, input *AccessIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Create(ctx context.Context, input *CreateIn, name string, out *CreateOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Tmpfile(ctx context.Context, input *CreateIn, out *CreateOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) OpenDir(ctx context.Context, input *OpenIn, out *OpenOut) (status syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Read(ctx context.Context, input *ReadIn, buf []byte) (ReadResult, syscall.Errno) {
	return nil, ENOSYS
}

func (fs *defaultRawFileSystem) GetLk(ctx context.Context, in *LkIn, out *LkOut) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) SetLk(ctx context.Context, in *LkIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) SetLkw(ctx context.Context, in *LkIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Release(ctx context.Context, input *ReleaseIn) {
}

func (fs *defaultRawFileSystem) Write(ctx context.Context, input *WriteIn, data []byte) (written uint32, code syscall.Errno) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) Writev(ctx context.Context, input *WriteIn, data [][]byte) (written uint32, code syscall.Errno) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) Flush(ctx context.Context, input *FlushIn) syscall.Errno {
	return OK
}

func (fs *defaultRawFileSystem) Fsync(ctx context.Context, input *FsyncIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDir(ctx context.Context, input *ReadIn, l *DirEntryList) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReadDirPlus(ctx context.Context, input *ReadIn, l *DirEntryList) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) ReleaseDir(ctx context.Context, input *ReleaseIn) {
}

func (fs *defaultRawFileSystem) FsyncDir(ctx context.Context, input *FsyncIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Fallocate(ctx context.Context, in *FallocateIn) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) CopyFileRange(ctx context.Context, input *CopyFileRangeIn) (written uint32, code syscall.Errno) {
	return 0, ENOSYS
}

func (fs *defaultRawFileSystem) Ioctl(ctx context.Context, input *IoctlIn, inbuf []byte, output *IoctlOut, outbuf []byte) (code syscall.Errno) {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Lseek(ctx context.Context, in *LseekIn, out *LseekOut) syscall.Errno {
	return ENOSYS
}

func (fs *defaultRawFileSystem) Statx(ctx context.Context, input *StatxIn, out *StatxOut) (code syscall.Errno) {
	return ENOSYS
}
