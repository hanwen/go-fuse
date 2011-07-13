package fuse

import (
	"time"
)

// TimingRawFileSystem is a wrapper to collect timings for a RawFileSystem
type TimingRawFileSystem struct {
	RawFileSystem

	*LatencyMap
}

func NewTimingRawFileSystem(fs RawFileSystem) *TimingRawFileSystem {
	t := new(TimingRawFileSystem)
	t.RawFileSystem = fs
	t.LatencyMap = NewLatencyMap()
	return t
}

func (me *TimingRawFileSystem) startTimer(name string) (closure func()) {
	start := time.Nanoseconds()

	return func() {
		dt := (time.Nanoseconds() - start) / 1e6
		me.LatencyMap.Add(name, "", dt)
	}
}

func (me *TimingRawFileSystem) Latencies() map[string]float64 {
	return me.LatencyMap.Latencies(1e-3)
}

func (me *TimingRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Lookup")()
	return me.RawFileSystem.Lookup(h, name)
}

func (me *TimingRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
	defer me.startTimer("Forget")()
	me.RawFileSystem.Forget(h, input)
}

func (me *TimingRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("GetAttr")()
	return me.RawFileSystem.GetAttr(header, input)
}

func (me *TimingRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("Open")()
	return me.RawFileSystem.Open(header, input)
}

func (me *TimingRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("SetAttr")()
	return me.RawFileSystem.SetAttr(header, input)
}

func (me *TimingRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	defer me.startTimer("Readlink")()
	return me.RawFileSystem.Readlink(header)
}

func (me *TimingRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mknod")()
	return me.RawFileSystem.Mknod(header, input, name)
}

func (me *TimingRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mkdir")()
	return me.RawFileSystem.Mkdir(header, input, name)
}

func (me *TimingRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	defer me.startTimer("Unlink")()
	return me.RawFileSystem.Unlink(header, name)
}

func (me *TimingRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	defer me.startTimer("Rmdir")()
	return me.RawFileSystem.Rmdir(header, name)
}

func (me *TimingRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	defer me.startTimer("Symlink")()
	return me.RawFileSystem.Symlink(header, pointedTo, linkName)
}

func (me *TimingRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	defer me.startTimer("Rename")()
	return me.RawFileSystem.Rename(header, input, oldName, newName)
}

func (me *TimingRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Link")()
	return me.RawFileSystem.Link(header, input, name)
}

func (me *TimingRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	defer me.startTimer("SetXAttr")()
	return me.RawFileSystem.SetXAttr(header, input, attr, data)
}

func (me *TimingRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	defer me.startTimer("GetXAttr")()
	return me.RawFileSystem.GetXAttr(header, attr)
}

func (me *TimingRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	defer me.startTimer("ListXAttr")()
	return me.RawFileSystem.ListXAttr(header)
}

func (me *TimingRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	defer me.startTimer("RemoveXAttr")()
	return me.RawFileSystem.RemoveXAttr(header, attr)
}

func (me *TimingRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	defer me.startTimer("Access")()
	return me.RawFileSystem.Access(header, input)
}

func (me *TimingRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	defer me.startTimer("Create")()
	return me.RawFileSystem.Create(header, input, name)
}

func (me *TimingRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("OpenDir")()
	return me.RawFileSystem.OpenDir(header, input)
}

func (me *TimingRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("Release")()
	me.RawFileSystem.Release(header, input)
}

func (me *TimingRawFileSystem) Read(input *ReadIn, bp BufferPool) ([]byte, Status) {
	defer me.startTimer("Read")()
	return me.RawFileSystem.Read(input, bp)
}

func (me *TimingRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	defer me.startTimer("Write")()
	return me.RawFileSystem.Write(input, data)
}

func (me *TimingRawFileSystem) Flush(input *FlushIn) Status {
	defer me.startTimer("Flush")()
	return me.RawFileSystem.Flush(input)
}

func (me *TimingRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	defer me.startTimer("Fsync")()
	return me.RawFileSystem.Fsync(input)
}

func (me *TimingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	defer me.startTimer("ReadDir")()
	return me.RawFileSystem.ReadDir(header, input)
}

func (me *TimingRawFileSystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("ReleaseDir")()
	me.RawFileSystem.ReleaseDir(header, input)
}

func (me *TimingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	defer me.startTimer("FsyncDir")()
	return me.RawFileSystem.FsyncDir(header, input)
}
