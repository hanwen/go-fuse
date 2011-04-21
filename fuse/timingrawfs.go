package fuse

import (
	"sync"
	"time"
)

// TimingRawFilesystem is a wrapper to collect timings for a RawFilesystem
type TimingRawFilesystem struct {
	WrappingRawFilesystem

	statisticsLock sync.Mutex
	latencies      map[string]int64
	counts         map[string]int64
}

func NewTimingRawFilesystem(fs RawFileSystem) *TimingRawFilesystem {
	t := new(TimingRawFilesystem)
	t.Original = fs
	t.latencies = make(map[string]int64)
	t.counts = make(map[string]int64)
	return t
}

func (me *TimingRawFilesystem) startTimer(name string) (closure func()) {
	start := time.Nanoseconds()

	return func() {
		dt := (time.Nanoseconds() - start) / 1e6
		me.statisticsLock.Lock()
		defer me.statisticsLock.Unlock()

		me.counts[name] += 1
		me.latencies[name] += dt
	}
}

func (me *TimingRawFilesystem) Latencies() map[string]float64 {
	me.statisticsLock.Lock()
	defer me.statisticsLock.Unlock()

	r := make(map[string]float64)
	for k, v := range me.counts {
		r[k] = float64(me.latencies[k]) / float64(v)
	}
	return r
}

func (me *TimingRawFilesystem) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	defer me.startTimer("Init")()
	return me.Original.Init(h, input)
}

func (me *TimingRawFilesystem) Destroy(h *InHeader, input *InitIn) {
	defer me.startTimer("Destroy")()
	me.Original.Destroy(h, input)
}

func (me *TimingRawFilesystem) Lookup(h *InHeader, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Lookup")()
	return me.Original.Lookup(h, name)
}

func (me *TimingRawFilesystem) Forget(h *InHeader, input *ForgetIn) {
	defer me.startTimer("Forget")()
	me.Original.Forget(h, input)
}

func (me *TimingRawFilesystem) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("GetAttr")()
	return me.Original.GetAttr(header, input)
}

func (me *TimingRawFilesystem) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("Open")()
	return me.Original.Open(header, input)
}

func (me *TimingRawFilesystem) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	defer me.startTimer("SetAttr")()
	return me.Original.SetAttr(header, input)
}

func (me *TimingRawFilesystem) Readlink(header *InHeader) (out []byte, code Status) {
	defer me.startTimer("Readlink")()
	return me.Original.Readlink(header)
}

func (me *TimingRawFilesystem) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mknod")()
	return me.Original.Mknod(header, input, name)
}

func (me *TimingRawFilesystem) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Mkdir")()
	return me.Original.Mkdir(header, input, name)
}

func (me *TimingRawFilesystem) Unlink(header *InHeader, name string) (code Status) {
	defer me.startTimer("Unlink")()
	return me.Original.Unlink(header, name)
}

func (me *TimingRawFilesystem) Rmdir(header *InHeader, name string) (code Status) {
	defer me.startTimer("Rmdir")()
	return me.Original.Rmdir(header, name)
}

func (me *TimingRawFilesystem) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	defer me.startTimer("Symlink")()
	return me.Original.Symlink(header, pointedTo, linkName)
}

func (me *TimingRawFilesystem) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	defer me.startTimer("Rename")()
	return me.Original.Rename(header, input, oldName, newName)
}

func (me *TimingRawFilesystem) Link(header *InHeader, input *LinkIn, name string) (out *EntryOut, code Status) {
	defer me.startTimer("Link")()
	return me.Original.Link(header, input, name)
}

func (me *TimingRawFilesystem) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	defer me.startTimer("SetXAttr")()
	return me.Original.SetXAttr(header, input, attr, data)
}

func (me *TimingRawFilesystem) GetXAttr(header *InHeader, attr string) (data []byte, code Status) {
	defer me.startTimer("GetXAttr")()
	return me.Original.GetXAttr(header, attr)
}

func (me *TimingRawFilesystem) ListXAttr(header *InHeader) (data []byte, code Status) {
	defer me.startTimer("ListXAttr")()
	return me.Original.ListXAttr(header)
}

func (me *TimingRawFilesystem) RemoveXAttr(header *InHeader, attr string) Status {
	defer me.startTimer("RemoveXAttr")()
	return me.Original.RemoveXAttr(header, attr)
}

func (me *TimingRawFilesystem) Access(header *InHeader, input *AccessIn) (code Status) {
	defer me.startTimer("Access")()
	return me.Original.Access(header, input)
}

func (me *TimingRawFilesystem) Create(header *InHeader, input *CreateIn, name string) (flags uint32, handle uint64, out *EntryOut, code Status) {
	defer me.startTimer("Create")()
	return me.Original.Create(header, input, name)
}

func (me *TimingRawFilesystem) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	defer me.startTimer("Bmap")()
	return me.Original.Bmap(header, input)
}

func (me *TimingRawFilesystem) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	defer me.startTimer("Ioctl")()
	return me.Original.Ioctl(header, input)
}

func (me *TimingRawFilesystem) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	defer me.startTimer("Poll")()
	return me.Original.Poll(header, input)
}

func (me *TimingRawFilesystem) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	defer me.startTimer("OpenDir")()
	return me.Original.OpenDir(header, input)
}

func (me *TimingRawFilesystem) Release(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("Release")()
	me.Original.Release(header, input)
}

func (me *TimingRawFilesystem) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	defer me.startTimer("Read")()
	return me.Original.Read(input, bp)
}

func (me *TimingRawFilesystem) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	defer me.startTimer("Write")()
	return me.Original.Write(input, data)
}

func (me *TimingRawFilesystem) Flush(input *FlushIn) Status {
	defer me.startTimer("Flush")()
	return me.Original.Flush(input)
}

func (me *TimingRawFilesystem) Fsync(input *FsyncIn) (code Status) {
	defer me.startTimer("Fsync")()
	return me.Original.Fsync(input)
}

func (me *TimingRawFilesystem) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	defer me.startTimer("ReadDir")()
	return me.Original.ReadDir(header, input)
}

func (me *TimingRawFilesystem) ReleaseDir(header *InHeader, input *ReleaseIn) {
	defer me.startTimer("ReleaseDir")()
	me.Original.ReleaseDir(header, input)
}

func (me *TimingRawFilesystem) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	defer me.startTimer("FsyncDir")()
	return me.Original.FsyncDir(header, input)
}
