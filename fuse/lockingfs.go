// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"fmt"
	"sync"
)

////////////////////////////////////////////////////////////////
// Locking raw FS.

type lockingRawFileSystem struct {
	RawFS RawFileSystem
	lock  sync.Mutex
}

// Returns a Wrap
func NewLockingRawFileSystem(fs RawFileSystem) RawFileSystem {
	return &lockingRawFileSystem{
		RawFS: fs,
	}
}

func (fs *lockingRawFileSystem) FS() RawFileSystem {
	return fs.RawFS
}

func (fs *lockingRawFileSystem) locked() func() {
	fs.lock.Lock()
	return func() { fs.lock.Unlock() }
}

func (fs *lockingRawFileSystem) Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Lookup(cancel, header, name, out)
}

func (fs *lockingRawFileSystem) SetDebug(dbg bool) {
	defer fs.locked()()
	fs.RawFS.SetDebug(dbg)
}

func (fs *lockingRawFileSystem) Forget(nodeID uint64, nlookup uint64) {
	defer fs.locked()()
	fs.RawFS.Forget(nodeID, nlookup)
}

func (fs *lockingRawFileSystem) GetAttr(cancel <-chan struct{}, input *GetAttrIn, out *AttrOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.GetAttr(cancel, input, out)
}

func (fs *lockingRawFileSystem) Open(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status) {

	defer fs.locked()()
	return fs.RawFS.Open(cancel, input, out)
}

func (fs *lockingRawFileSystem) SetAttr(cancel <-chan struct{}, input *SetAttrIn, out *AttrOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetAttr(cancel, input, out)
}

func (fs *lockingRawFileSystem) Readlink(cancel <-chan struct{}, header *InHeader) (out []byte, code Status) {
	defer fs.locked()()
	return fs.RawFS.Readlink(cancel, header)
}

func (fs *lockingRawFileSystem) Mknod(cancel <-chan struct{}, input *MknodIn, name string, out *EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mknod(cancel, input, name, out)
}

func (fs *lockingRawFileSystem) Mkdir(cancel <-chan struct{}, input *MkdirIn, name string, out *EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Mkdir(cancel, input, name, out)
}

func (fs *lockingRawFileSystem) Unlink(cancel <-chan struct{}, header *InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Unlink(cancel, header, name)
}

func (fs *lockingRawFileSystem) Rmdir(cancel <-chan struct{}, header *InHeader, name string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rmdir(cancel, header, name)
}

func (fs *lockingRawFileSystem) Symlink(cancel <-chan struct{}, header *InHeader, pointedTo string, linkName string, out *EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Symlink(cancel, header, pointedTo, linkName, out)
}

func (fs *lockingRawFileSystem) Rename(cancel <-chan struct{}, input *RenameIn, oldName string, newName string) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Rename(cancel, input, oldName, newName)
}

func (fs *lockingRawFileSystem) Link(cancel <-chan struct{}, input *LinkIn, name string, out *EntryOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Link(cancel, input, name, out)
}

func (fs *lockingRawFileSystem) SetXAttr(cancel <-chan struct{}, input *SetXAttrIn, attr string, data []byte) Status {
	defer fs.locked()()
	return fs.RawFS.SetXAttr(cancel, input, attr, data)
}

func (fs *lockingRawFileSystem) GetXAttr(cancel <-chan struct{}, header *InHeader, attr string, dest []byte) (uint32, Status) {
	defer fs.locked()()
	return fs.RawFS.GetXAttr(cancel, header, attr, dest)
}

func (fs *lockingRawFileSystem) ListXAttr(cancel <-chan struct{}, header *InHeader, data []byte) (uint32, Status) {
	defer fs.locked()()
	return fs.RawFS.ListXAttr(cancel, header, data)
}

func (fs *lockingRawFileSystem) RemoveXAttr(cancel <-chan struct{}, header *InHeader, attr string) Status {
	defer fs.locked()()
	return fs.RawFS.RemoveXAttr(cancel, header, attr)
}

func (fs *lockingRawFileSystem) Access(cancel <-chan struct{}, input *AccessIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Access(cancel, input)
}

func (fs *lockingRawFileSystem) Create(cancel <-chan struct{}, input *CreateIn, name string, out *CreateOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Create(cancel, input, name, out)
}

func (fs *lockingRawFileSystem) OpenDir(cancel <-chan struct{}, input *OpenIn, out *OpenOut) (status Status) {
	defer fs.locked()()
	return fs.RawFS.OpenDir(cancel, input, out)
}

func (fs *lockingRawFileSystem) Release(input *ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.Release(input)
}

func (fs *lockingRawFileSystem) ReleaseDir(input *ReleaseIn) {
	defer fs.locked()()
	fs.RawFS.ReleaseDir(input)
}

func (fs *lockingRawFileSystem) Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status) {
	defer fs.locked()()
	return fs.RawFS.Read(cancel, input, buf)
}

func (fs *lockingRawFileSystem) GetLk(cancel <-chan struct{}, in *LkIn, out *LkOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.GetLk(cancel, in, out)
}

func (fs *lockingRawFileSystem) SetLk(cancel <-chan struct{}, in *LkIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetLk(cancel, in)
}

func (fs *lockingRawFileSystem) SetLkw(cancel <-chan struct{}, in *LkIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.SetLkw(cancel, in)
}

func (fs *lockingRawFileSystem) Write(cancel <-chan struct{}, input *WriteIn, data []byte) (written uint32, code Status) {
	defer fs.locked()()
	return fs.RawFS.Write(cancel, input, data)
}

func (fs *lockingRawFileSystem) Flush(cancel <-chan struct{}, input *FlushIn) Status {
	defer fs.locked()()
	return fs.RawFS.Flush(cancel, input)
}

func (fs *lockingRawFileSystem) Fsync(cancel <-chan struct{}, input *FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fsync(cancel, input)
}

func (fs *lockingRawFileSystem) ReadDir(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDir(cancel, input, out)
}

func (fs *lockingRawFileSystem) ReadDirPlus(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status {
	defer fs.locked()()
	return fs.RawFS.ReadDirPlus(cancel, input, out)
}

func (fs *lockingRawFileSystem) FsyncDir(cancel <-chan struct{}, input *FsyncIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.FsyncDir(cancel, input)
}

func (fs *lockingRawFileSystem) Init(s *Server) {
	defer fs.locked()()
	fs.RawFS.Init(s)
}

func (fs *lockingRawFileSystem) StatFs(cancel <-chan struct{}, header *InHeader, out *StatfsOut) (code Status) {
	defer fs.locked()()
	return fs.RawFS.StatFs(cancel, header, out)
}

func (fs *lockingRawFileSystem) Fallocate(cancel <-chan struct{}, in *FallocateIn) (code Status) {
	defer fs.locked()()
	return fs.RawFS.Fallocate(cancel, in)
}

func (fs *lockingRawFileSystem) String() string {
	defer fs.locked()()
	return fmt.Sprintf("Locked(%s)", fs.RawFS.String())
}
