package fuse

import (
	"sync"
	"time"
	"log"
	"fmt"
	"sort"
)

var _ = log.Print
var _ = fmt.Print

// TimingPathFilesystem is a wrapper to collect timings for a PathFilesystem
type TimingPathFilesystem struct {
	WrappingPathFilesystem

	statisticsLock sync.Mutex
	latencies      map[string]int64
	counts         map[string]int64
	pathCounts     map[string]map[string]int64
}

func NewTimingPathFilesystem(fs PathFilesystem) *TimingPathFilesystem {
	t := new(TimingPathFilesystem)
	t.Original = fs
	t.latencies = make(map[string]int64)
	t.counts = make(map[string]int64)
	t.pathCounts = make(map[string]map[string]int64)
	return t
}

func (me *TimingPathFilesystem) startTimer(name string, arg string) (closure func()) {
	start := time.Nanoseconds()

	return func() {
		dt := (time.Nanoseconds() - start) / 1e6
		me.statisticsLock.Lock()
		defer me.statisticsLock.Unlock()

		me.counts[name] += 1
		me.latencies[name] += dt

		m, ok := me.pathCounts[name]
		if !ok {
			m = make(map[string]int64)
			me.pathCounts[name] = m
		}
		m[arg] += 1
	}
}

func (me *TimingPathFilesystem) OperationCounts() map[string]int64 {
	me.statisticsLock.Lock()
	defer me.statisticsLock.Unlock()

	r := make(map[string]int64)
	for k, v := range me.counts {
		r[k] = v
	}
	return r
}

func (me *TimingPathFilesystem) Latencies() map[string]float64 {
	me.statisticsLock.Lock()
	defer me.statisticsLock.Unlock()

	r := make(map[string]float64)
	for k, v := range me.counts {
		r[k] = float64(me.latencies[k]) / float64(v)
	}
	return r
}

func (me *TimingPathFilesystem) HotPaths(operation string) (paths []string, uniquePaths int) {
	me.statisticsLock.Lock()
	defer me.statisticsLock.Unlock()

	counts := me.pathCounts[operation]
	results := make([]string, 0, len(counts))
	for k, v := range counts {
		results = append(results, fmt.Sprintf("% 9d %s", v, k))

	}
	sort.SortStrings(results)
	return results, len(counts)
}

func (me *TimingPathFilesystem) GetAttr(name string) (*Attr, Status) {
	defer me.startTimer("GetAttr", name)()
	return me.Original.GetAttr(name)
}

func (me *TimingPathFilesystem) GetXAttr(name string, attr string) ([]byte, Status) {
	defer me.startTimer("GetXAttr", name)()
	return me.Original.GetXAttr(name, attr)
}

func (me *TimingPathFilesystem) SetXAttr(name string, attr string, data []byte, flags int) Status {
	defer me.startTimer("SetXAttr", name)()
	return me.Original.SetXAttr(name, attr, data, flags)
}

func (me *TimingPathFilesystem) ListXAttr(name string) ([]string, Status) {
	defer me.startTimer("ListXAttr", name)()
	return me.Original.ListXAttr(name)
}

func (me *TimingPathFilesystem) RemoveXAttr(name string, attr string) Status {
	defer me.startTimer("RemoveXAttr", name)()
	return me.Original.RemoveXAttr(name, attr)
}

func (me *TimingPathFilesystem) Readlink(name string) (string, Status) {
	defer me.startTimer("Readlink", name)()
	return me.Original.Readlink(name)
}

func (me *TimingPathFilesystem) Mknod(name string, mode uint32, dev uint32) Status {
	defer me.startTimer("Mknod", name)()
	return me.Original.Mknod(name, mode, dev)
}

func (me *TimingPathFilesystem) Mkdir(name string, mode uint32) Status {
	defer me.startTimer("Mkdir", name)()
	return me.Original.Mkdir(name, mode)
}

func (me *TimingPathFilesystem) Unlink(name string) (code Status) {
	defer me.startTimer("Unlink", name)()
	return me.Original.Unlink(name)
}

func (me *TimingPathFilesystem) Rmdir(name string) (code Status) {
	defer me.startTimer("Rmdir", name)()
	return me.Original.Rmdir(name)
}

func (me *TimingPathFilesystem) Symlink(value string, linkName string) (code Status) {
	defer me.startTimer("Symlink", linkName)()
	return me.Original.Symlink(value, linkName)
}

func (me *TimingPathFilesystem) Rename(oldName string, newName string) (code Status) {
	defer me.startTimer("Rename", oldName)()
	return me.Original.Rename(oldName, newName)
}

func (me *TimingPathFilesystem) Link(oldName string, newName string) (code Status) {
	defer me.startTimer("Link", newName)()
	return me.Original.Link(oldName, newName)
}

func (me *TimingPathFilesystem) Chmod(name string, mode uint32) (code Status) {
	defer me.startTimer("Chmod", name)()
	return me.Original.Chmod(name, mode)
}

func (me *TimingPathFilesystem) Chown(name string, uid uint32, gid uint32) (code Status) {
	defer me.startTimer("Chown", name)()
	return me.Original.Chown(name, uid, gid)
}

func (me *TimingPathFilesystem) Truncate(name string, offset uint64) (code Status) {
	defer me.startTimer("Truncate", name)()
	return me.Original.Truncate(name, offset)
}

func (me *TimingPathFilesystem) Open(name string, flags uint32) (file RawFuseFile, code Status) {
	defer me.startTimer("Open", name)()
	return me.Original.Open(name, flags)
}

func (me *TimingPathFilesystem) OpenDir(name string) (stream chan DirEntry, status Status) {
	defer me.startTimer("OpenDir", name)()
	return me.Original.OpenDir(name)
}

func (me *TimingPathFilesystem) Mount(conn *PathFileSystemConnector) Status {
	defer me.startTimer("Mount", "")()
	return me.Original.Mount(conn)
}

func (me *TimingPathFilesystem) Unmount() {
	defer me.startTimer("Unmount", "")()
	me.Original.Unmount()
}

func (me *TimingPathFilesystem) Access(name string, mode uint32) (code Status) {
	defer me.startTimer("Access", name)()
	return me.Original.Access(name, mode)
}

func (me *TimingPathFilesystem) Create(name string, flags uint32, mode uint32) (file RawFuseFile, code Status) {
	defer me.startTimer("Create", name)()
	return me.Original.Create(name, flags, mode)
}

func (me *TimingPathFilesystem) Utimens(name string, AtimeNs uint64, CtimeNs uint64) (code Status) {
	defer me.startTimer("Utimens", name)()
	return me.Original.Utimens(name, AtimeNs, CtimeNs)
}
