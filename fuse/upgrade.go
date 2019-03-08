// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
)

// NewRawFileSystem adds the methods missing for implementing a
// RawFileSystem to any object.
func NewRawFileSystem(fs interface{}) RawFileSystem {
	return &wrappingFS{fs}
}

type wrappingFS struct {
	fs interface{}
}

func (fs *wrappingFS) Init(srv *Server) {
	if s, ok := fs.fs.(interface {
		Init(*Server)
	}); ok {
		s.Init(srv)
	}
}

func (fs *wrappingFS) String() string {
	return fmt.Sprintf("%v", fs.fs)
}

func (fs *wrappingFS) SetDebug(dbg bool) {
	if s, ok := fs.fs.(interface {
		SetDebug(bool)
	}); ok {
		s.SetDebug(dbg)
	}
}

func (fs *wrappingFS) StatFs(cancel <-chan struct{}, header *InHeader, out *StatfsOut) Status {
	if s, ok := fs.fs.(interface {
		StatFs(header *InHeader, out *StatfsOut) Status
	}); ok {
		return s.StatFs(header, out)
	}
	if s, ok := fs.fs.(interface {
		StatFs(cancel <-chan struct{}, header *InHeader, out *StatfsOut) Status
	}); ok {
		return s.StatFs(cancel, header, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Lookup(header *InHeader, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Lookup(header, name, out)
	}
	if s, ok := fs.fs.(interface {
		Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Lookup(cancel, header, name, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Forget(nodeID, nlookup uint64) {
	if s, ok := fs.fs.(interface {
		Forget(nodeID, nlookup uint64)
	}); ok {
		s.Forget(nodeID, nlookup)
	}
}

func (fs *wrappingFS) GetAttr(cancel <-chan struct{}, input *GetAttrIn, out *AttrOut) (code Status) {
	if s, ok := fs.fs.(interface {
		GetAttr(input *GetAttrIn, out *AttrOut) (code Status)
	}); ok {
		return s.GetAttr(input, out)
	}
	if s, ok := fs.fs.(interface {
		GetAttr(cancel <-chan struct{}, input *GetAttrIn, out *AttrOut) (code Status)
	}); ok {
		return s.GetAttr(cancel, input, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Open(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status) {
	if s, ok := fs.fs.(interface {
		Open(input *OpenIn, out *OpenOut) (status Status)
	}); ok {
		return s.Open(input, out)
	}
	if s, ok := fs.fs.(interface {
		Open(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status)
	}); ok {
		return s.Open(cancel, input, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) SetAttr(cancel <-chan struct{}, input *SetAttrIn, out *AttrOut) (code Status) {
	if s, ok := fs.fs.(interface {
		SetAttr(input *SetAttrIn, out *AttrOut) (code Status)
	}); ok {
		return s.SetAttr(input, out)
	}
	if s, ok := fs.fs.(interface {
		SetAttr(cancel <-chan struct{}, input *SetAttrIn, out *AttrOut) (code Status)
	}); ok {
		return s.SetAttr(cancel, input, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Readlink(cancel <-chan struct{}, header *InHeader) (out []byte, code Status) {
	if s, ok := fs.fs.(interface {
		Readlink(header *InHeader) (out []byte, code Status)
	}); ok {
		return s.Readlink(header)
	}
	if s, ok := fs.fs.(interface {
		Readlink(cancel <-chan struct{}, header *InHeader) (out []byte, code Status)
	}); ok {
		return s.Readlink(cancel, header)
	}
	return nil, ENOSYS
}

func (fs *wrappingFS) Mknod(cancel <-chan struct{}, input *MknodIn, name string, out *EntryOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Mknod(input *MknodIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Mknod(input, name, out)
	}
	if s, ok := fs.fs.(interface {
		Mknod(cancel <-chan struct{}, input *MknodIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Mknod(cancel, input, name, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Mkdir(cancel <-chan struct{}, input *MkdirIn, name string, out *EntryOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Mkdir(input *MkdirIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Mkdir(input, name, out)
	}
	if s, ok := fs.fs.(interface {
		Mkdir(cancel <-chan struct{}, input *MkdirIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Mkdir(cancel, input, name, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Unlink(cancel <-chan struct{}, header *InHeader, name string) (code Status) {
	if s, ok := fs.fs.(interface {
		Unlink(header *InHeader, name string) (code Status)
	}); ok {
		return s.Unlink(header, name)
	}
	if s, ok := fs.fs.(interface {
		Unlink(cancel <-chan struct{}, header *InHeader, name string) (code Status)
	}); ok {
		return s.Unlink(cancel, header, name)
	}
	return ENOSYS
}

func (fs *wrappingFS) Rmdir(cancel <-chan struct{}, header *InHeader, name string) (code Status) {
	if s, ok := fs.fs.(interface {
		Rmdir(header *InHeader, name string) (code Status)
	}); ok {
		return s.Rmdir(header, name)
	}
	if s, ok := fs.fs.(interface {
		Rmdir(cancel <-chan struct{}, header *InHeader, name string) (code Status)
	}); ok {
		return s.Rmdir(cancel, header, name)
	}
	return ENOSYS
}

func (fs *wrappingFS) Symlink(cancel <-chan struct{}, header *InHeader, pointedTo string, linkName string, out *EntryOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Symlink(header *InHeader, pointedTo string, linkName string, out *EntryOut) (code Status)
	}); ok {
		return s.Symlink(header, pointedTo, linkName, out)
	}
	if s, ok := fs.fs.(interface {
		Symlink(cancel <-chan struct{}, header *InHeader, pointedTo string, linkName string, out *EntryOut) (code Status)
	}); ok {
		return s.Symlink(cancel, header, pointedTo, linkName, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Rename(cancel <-chan struct{}, input *RenameIn, oldName string, newName string) (code Status) {
	if s, ok := fs.fs.(interface {
		Rename(input *RenameIn, oldName string, newName string) (code Status)
	}); ok {
		return s.Rename(input, oldName, newName)
	}
	if s, ok := fs.fs.(interface {
		Rename(cancel <-chan struct{}, input *RenameIn, oldName string, newName string) (code Status)
	}); ok {
		return s.Rename(cancel, input, oldName, newName)
	}
	return ENOSYS
}

func (fs *wrappingFS) Link(cancel <-chan struct{}, input *LinkIn, name string, out *EntryOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Link(input *LinkIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Link(input, name, out)
	}
	if s, ok := fs.fs.(interface {
		Link(cancel <-chan struct{}, input *LinkIn, name string, out *EntryOut) (code Status)
	}); ok {
		return s.Link(cancel, input, name, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) GetXAttrSize(cancel <-chan struct{}, header *InHeader, attr string) (size int, code Status) {
	if s, ok := fs.fs.(interface {
		GetXAttrSize(header *InHeader, attr string) (size int, code Status)
	}); ok {
		return s.GetXAttrSize(header, attr)
	}
	if s, ok := fs.fs.(interface {
		GetXAttrSize(cancel <-chan struct{}, header *InHeader, attr string) (size int, code Status)
	}); ok {
		return s.GetXAttrSize(cancel, header, attr)
	}
	return 0, ENOSYS
}

func (fs *wrappingFS) GetXAttrData(cancel <-chan struct{}, header *InHeader, attr string) (data []byte, code Status) {
	if s, ok := fs.fs.(interface {
		GetXAttrData(header *InHeader, attr string) (data []byte, code Status)
	}); ok {
		return s.GetXAttrData(header, attr)
	}
	if s, ok := fs.fs.(interface {
		GetXAttrData(cancel <-chan struct{}, header *InHeader, attr string) (data []byte, code Status)
	}); ok {
		return s.GetXAttrData(cancel, header, attr)
	}
	return nil, ENOSYS
}

func (fs *wrappingFS) SetXAttr(cancel <-chan struct{}, input *SetXAttrIn, attr string, data []byte) Status {
	if s, ok := fs.fs.(interface {
		SetXAttr(input *SetXAttrIn, attr string, data []byte) Status
	}); ok {
		return s.SetXAttr(input, attr, data)
	}
	if s, ok := fs.fs.(interface {
		SetXAttr(cancel <-chan struct{}, input *SetXAttrIn, attr string, data []byte) Status
	}); ok {
		return s.SetXAttr(cancel, input, attr, data)
	}
	return ENOSYS
}

func (fs *wrappingFS) ListXAttr(cancel <-chan struct{}, header *InHeader) (data []byte, code Status) {
	if s, ok := fs.fs.(interface {
		ListXAttr(header *InHeader) (data []byte, code Status)
	}); ok {
		return s.ListXAttr(header)
	}
	if s, ok := fs.fs.(interface {
		ListXAttr(cancel <-chan struct{}, header *InHeader) (data []byte, code Status)
	}); ok {
		return s.ListXAttr(cancel, header)
	}
	return nil, ENOSYS
}

func (fs *wrappingFS) RemoveXAttr(cancel <-chan struct{}, header *InHeader, attr string) Status {
	if s, ok := fs.fs.(interface {
		RemoveXAttr(header *InHeader, attr string) Status
	}); ok {
		return s.RemoveXAttr(header, attr)
	}
	if s, ok := fs.fs.(interface {
		RemoveXAttr(cancel <-chan struct{}, header *InHeader, attr string) Status
	}); ok {
		return s.RemoveXAttr(cancel, header, attr)
	}
	return ENOSYS
}

func (fs *wrappingFS) Access(cancel <-chan struct{}, input *AccessIn) (code Status) {
	if s, ok := fs.fs.(interface {
		Access(input *AccessIn) (code Status)
	}); ok {
		return s.Access(input)
	}
	if s, ok := fs.fs.(interface {
		Access(cancel <-chan struct{}, input *AccessIn) (code Status)
	}); ok {
		return s.Access(cancel, input)
	}
	return ENOSYS
}

func (fs *wrappingFS) Create(cancel <-chan struct{}, input *CreateIn, name string, out *CreateOut) (code Status) {
	if s, ok := fs.fs.(interface {
		Create(input *CreateIn, name string, out *CreateOut) (code Status)
	}); ok {
		return s.Create(input, name, out)
	}
	if s, ok := fs.fs.(interface {
		Create(cancel <-chan struct{}, input *CreateIn, name string, out *CreateOut) (code Status)
	}); ok {
		return s.Create(cancel, input, name, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) OpenDir(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status) {
	if s, ok := fs.fs.(interface {
		OpenDir(input *OpenIn, out *OpenOut) (status Status)
	}); ok {
		return s.OpenDir(input, out)
	}
	if s, ok := fs.fs.(interface {
		OpenDir(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status)
	}); ok {
		return s.OpenDir(cancel, input, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status) {
	if s, ok := fs.fs.(interface {
		Read(input *ReadIn, buf []byte) (ReadResult, Status)
	}); ok {
		return s.Read(input, buf)
	}
	if s, ok := fs.fs.(interface {
		Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status)
	}); ok {
		return s.Read(cancel, input, buf)
	}
	return nil, ENOSYS
}

func (fs *wrappingFS) GetLk(cancel <-chan struct{}, in *LkIn, out *LkOut) (code Status) {
	if s, ok := fs.fs.(interface {
		GetLk(in *LkIn, out *LkOut) (code Status)
	}); ok {
		return s.GetLk(in, out)
	}
	if s, ok := fs.fs.(interface {
		GetLk(cancel <-chan struct{}, in *LkIn, out *LkOut) (code Status)
	}); ok {
		return s.GetLk(cancel, in, out)
	}
	return ENOSYS
}

func (fs *wrappingFS) SetLk(cancel <-chan struct{}, in *LkIn) (code Status) {
	if s, ok := fs.fs.(interface {
		SetLk(in *LkIn) (code Status)
	}); ok {
		return s.SetLk(in)
	}
	if s, ok := fs.fs.(interface {
		SetLk(cancel <-chan struct{}, in *LkIn) (code Status)
	}); ok {
		return s.SetLk(cancel, in)
	}
	return ENOSYS
}

func (fs *wrappingFS) SetLkw(cancel <-chan struct{}, in *LkIn) (code Status) {
	if s, ok := fs.fs.(interface {
		SetLkw(in *LkIn) (code Status)
	}); ok {
		return s.SetLkw(in)
	}
	if s, ok := fs.fs.(interface {
		SetLkw(cancel <-chan struct{}, in *LkIn) (code Status)
	}); ok {
		return s.SetLkw(cancel, in)
	}
	return ENOSYS
}

func (fs *wrappingFS) Release(input *ReleaseIn) {
	if s, ok := fs.fs.(interface {
		Release(input *ReleaseIn)
	}); ok {
		s.Release(input)
	}
}

func (fs *wrappingFS) Write(cancel <-chan struct{}, input *WriteIn, data []byte) (written uint32, code Status) {
	if s, ok := fs.fs.(interface {
		Write(input *WriteIn, data []byte) (written uint32, code Status)
	}); ok {
		return s.Write(input, data)
	}
	if s, ok := fs.fs.(interface {
		Write(cancel <-chan struct{}, input *WriteIn, data []byte) (written uint32, code Status)
	}); ok {
		return s.Write(cancel, input, data)
	}
	return 0, ENOSYS
}

func (fs *wrappingFS) Flush(cancel <-chan struct{}, input *FlushIn) Status {
	if s, ok := fs.fs.(interface {
		Flush(input *FlushIn) Status
	}); ok {
		return s.Flush(input)
	}
	if s, ok := fs.fs.(interface {
		Flush(cancel <-chan struct{}, input *FlushIn) Status
	}); ok {
		return s.Flush(cancel, input)
	}
	return OK
}

func (fs *wrappingFS) Fsync(cancel <-chan struct{}, input *FsyncIn) (code Status) {
	if s, ok := fs.fs.(interface {
		Fsync(input *FsyncIn) (code Status)
	}); ok {
		return s.Fsync(input)
	}
	if s, ok := fs.fs.(interface {
		Fsync(cancel <-chan struct{}, input *FsyncIn) (code Status)
	}); ok {
		return s.Fsync(cancel, input)
	}
	return ENOSYS
}

func (fs *wrappingFS) ReadDir(cancel <-chan struct{}, input *ReadIn, l *DirEntryList) Status {
	if s, ok := fs.fs.(interface {
		ReadDir(input *ReadIn, l *DirEntryList) Status
	}); ok {
		return s.ReadDir(input, l)
	}
	if s, ok := fs.fs.(interface {
		ReadDir(cancel <-chan struct{}, input *ReadIn, l *DirEntryList) Status
	}); ok {
		return s.ReadDir(cancel, input, l)
	}
	return ENOSYS
}

func (fs *wrappingFS) ReadDirPlus(cancel <-chan struct{}, input *ReadIn, l *DirEntryList) Status {
	if s, ok := fs.fs.(interface {
		ReadDirPlus(input *ReadIn, l *DirEntryList) Status
	}); ok {
		return s.ReadDirPlus(input, l)
	}
	if s, ok := fs.fs.(interface {
		ReadDirPlus(cancel <-chan struct{}, input *ReadIn, l *DirEntryList) Status
	}); ok {
		return s.ReadDirPlus(cancel, input, l)
	}
	return ENOSYS
}

func (fs *wrappingFS) ReleaseDir(input *ReleaseIn) {
	if s, ok := fs.fs.(interface {
		ReleaseDir(input *ReleaseIn)
	}); ok {
		s.ReleaseDir(input)
	}
}

func (fs *wrappingFS) FsyncDir(cancel <-chan struct{}, input *FsyncIn) (code Status) {
	if s, ok := fs.fs.(interface {
		FsyncDir(input *FsyncIn) (code Status)
	}); ok {
		return s.FsyncDir(input)
	}
	if s, ok := fs.fs.(interface {
		FsyncDir(cancel <-chan struct{}, input *FsyncIn) (code Status)
	}); ok {
		return s.FsyncDir(cancel, input)
	}
	return ENOSYS
}

func (fs *wrappingFS) Fallocate(cancel <-chan struct{}, in *FallocateIn) (code Status) {
	if s, ok := fs.fs.(interface {
		Fallocate(in *FallocateIn) (code Status)
	}); ok {
		return s.Fallocate(in)
	}
	if s, ok := fs.fs.(interface {
		Fallocate(cancel <-chan struct{}, in *FallocateIn) (code Status)
	}); ok {
		return s.Fallocate(cancel, in)
	}
	return ENOSYS
}
