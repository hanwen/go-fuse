package fuse

import (
	"time"
)

// TimingRawFileSystem is a wrapper to collect timings for a RawFileSystem
type TimingRawFileSystem struct {
	WrappingRawFileSystem

	*LatencyMap
}

func NewTimingRawFileSystem(fs RawFileSystem) *TimingRawFileSystem {
	t := new(TimingRawFileSystem)
	t.Original = fs
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

func (me *TimingRawFileSystem) Destroy(h *InHeader, input *InitIn) {
	defer me.startTimer("Destroy")()
	me.Original.Destroy(h, input)
}

func (me *TimingRawFileSystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Lookup")()
	return me.Original.Lookup(h, name)
}

func (me *TimingRawFileSystem) Forget(h *InHeader, input *ForgetIn) {
	defer me.startTimer("Forget")()
	me.Original.Forget(h, input)
}

func (me *TimingRawFileSystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("GetAttr")()
	return me.Original.GetAttr(header, input)
}

func (me *TimingRawFileSystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("Open")()
	return me.Original.Open(header, input)
}

func (me *TimingRawFileSystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("SetAttr")()
	return me.Original.SetAttr(header, input)
}

func (me *TimingRawFileSystem) Readlink(header *InHeader) (out []byte, code Status) {
	defer me.startTimer("Readlink")()
	return me.Original.Readlink(header)
}

func (me *TimingRawFileSystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mknod")()
	return me.Original.Mknod(header, input, name)
}

func (me *TimingRawFileSystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mkdir")()
	return me.Original.Mkdir(header, input, name)
}

func (me *TimingRawFileSystem) Unlink(header *InHeader, name string) (code Status) {
	defer me.startTimer("Unlink")()
	return me.Original.Unlink(header, name)
}

func (me *TimingRawFileSystem) Rmdir(header *InHeader, name string) (code Status) {
	defer me.startTimer("Rmdir")()
	return me.Original.Rmdir(header, name)
}

func (me *TimingRawFileSystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	defer me.startTimer("Symlink")()
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *TimingRawFileSystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	defer me.startTimer("Rename")()
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *TimingRawFileSystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Link")()
	return me.Original.Link(header, input, name)
}

func (me *TimingRawFileSystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	defer me.startTimer("SetXAttr")()
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *TimingRawFileSystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	defer me.startTimer("GetXAttr")()
	return me.Original.GetXAttr(header, attr)
}

func (me *TimingRawFileSystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	defer me.startTimer("ListXAttr")()
	return me.Original.ListXAttr(header)
}

func (me *TimingRawFileSystem) RemoveXAttr(header *InHeader, attr string) Status {
	defer me.startTimer("RemoveXAttr")()
	return me.Original.RemoveXAttr(header, attr)
}

func (me *TimingRawFileSystem) Access(header *InHeader, input *AccessIn) (code Status) {
	defer me.startTimer("Access")()
	return me.Original.Access(header, input)
}

func (me *TimingRawFileSystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	defer me.startTimer("Create")()
	return me.Original.Create(header, input, name)
}

func (me *TimingRawFileSystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("OpenDir")()
	return me.Original.OpenDir(header, input)
}

func (me *TimingRawFileSystem) Release(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("Release")()
	me.Original.Release(header, input)
}

func (me *TimingRawFileSystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	defer me.startTimer("Read")()
	return me.Original.Read(input, bp)
}

func (me *TimingRawFileSystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	defer me.startTimer("Write")()
	return me.Original.Write(input, data)
}

func (me *TimingRawFileSystem) Flush(input *FlushIn) Status {
	defer me.startTimer("Flush")()
	return me.Original.Flush(input)
}

func (me *TimingRawFileSystem) Fsync(input *FsyncIn) (code Status) {
	defer me.startTimer("Fsync")()
	return me.Original.Fsync(input)
}

func (me *TimingRawFileSystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	defer me.startTimer("ReadDir")()
	return me.Original.ReadDir(header, input)
}

func (me *TimingRawFileSystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("ReleaseDir")()
	me.Original.ReleaseDir(header, input)
}

func (me *TimingRawFileSystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	defer me.startTimer("FsyncDir")()
	return me.Original.FsyncDir(header, input)
}
