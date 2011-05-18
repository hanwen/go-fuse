package fuse

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

var _ = fmt.Println
var _ = log.Println

const (
	DebugDir = ".debug"
)

type getter func() []byte

// FileSystemDebug exposes a .debug directory, exposing files for
// which a read hooks into a callback.  This is useful for exporting
// metrics and debug information from the daemon.
//
// TODO - should use in-process mount instead?
type FileSystemDebug struct {
	sync.RWMutex
	callbacks map[string]getter
	FileSystem
}

func NewFileSystemDebug() *FileSystemDebug {
	return &FileSystemDebug{
		callbacks: make(map[string]getter),
	}
}

func (me *FileSystemDebug) Add(name string, callback getter) {
	me.RWMutex.Lock()
	defer me.RWMutex.Unlock()
	me.callbacks[name] = callback
}

func (me *FileSystemDebug) Open(path string, flags uint32) (fuseFile File, status Status) {
	content := me.getContent(path)
	if content != nil {
		return NewReadOnlyFile(content), OK
	}
	return me.FileSystem.Open(path, flags)
}

func (me *FileSystemDebug) getContent(path string) []byte {
	comps := strings.Split(path, filepath.SeparatorString, -1)
	if comps[0] == DebugDir {
		me.RWMutex.RLock()
		defer me.RWMutex.RUnlock()
		f := me.callbacks[comps[1]]
		if f != nil {
			return f()
		}
	}
	return nil
}

func (me *FileSystemDebug) GetXAttr(name string, attr string) ([]byte, Status) {
	if strings.HasPrefix(name, DebugDir) {
		return nil, syscall.ENODATA
	}
	return me.FileSystem.GetXAttr(name, attr)
}

func (me *FileSystemDebug) GetAttr(path string) (*os.FileInfo, Status) {
	if !strings.HasPrefix(path, DebugDir) {
		return me.FileSystem.GetAttr(path)
	}
	if path == DebugDir {
		return &os.FileInfo{
			Mode: S_IFDIR | 0755,
		},OK
	}
	c := me.getContent(path)
	if c != nil {
		return &os.FileInfo{
			Mode: S_IFREG | 0644,
			Size: int64(len(c)),
		},OK
	}
	return nil, ENOENT
}

func FloatMapToBytes(m map[string]float64) []byte {
	keys := make([]string, 0, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}

	sort.SortStrings(keys)

	var r []string
	for _, k := range keys {
		r = append(r, fmt.Sprintf("%v %v", k, m[k]))
	}
	return []byte(strings.Join(r, "\n"))
}

// Ugh - generics.
func IntMapToBytes(m map[string]int) []byte {
	keys := make([]string, 0, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}

	sort.SortStrings(keys)

	var r []string
	for _, k := range keys {
		r = append(r, fmt.Sprintf("%v %v", k, m[k]))
	}
	return []byte(strings.Join(r, "\n"))
}

func (me *FileSystemDebug) OpenDir(name string) (stream chan DirEntry, status Status) {
	if name == DebugDir {
		me.RWMutex.RLock()
		defer me.RWMutex.RUnlock()

		stream = make(chan DirEntry, len(me.callbacks))
		for k, _ := range me.callbacks {
			stream <- DirEntry{
				Name: k,
				Mode: S_IFREG,
			}
		}
		close(stream)
		return stream, OK
	}
	return me.FileSystem.OpenDir(name)
}

func (me *FileSystemDebug) AddMountState(state *MountState) {
	me.Add("mountstate-latencies",
		func() []byte { return FloatMapToBytes(state.Latencies()) })
	me.Add("mountstate-opcounts",
		func() []byte { return IntMapToBytes(state.OperationCounts()) })
	me.Add("mountstate-bufferpool",
		func() []byte { return []byte(state.BufferPoolStats()) })
}

func (me *FileSystemDebug) AddFileSystemConnector(conn *FileSystemConnector) {
	me.Add("filesystemconnector-stats",
		func() []byte { return []byte(conn.Statistics()) })
}

func hotPaths(timing *TimingFileSystem) []byte {
	hot := timing.HotPaths("GetAttr")
	unique := len(hot)
	top := 20
	start := len(hot) - top
	if start < 0 {
		start = 0
	}
	return []byte(fmt.Sprintf("Unique GetAttr paths: %d\nTop %d GetAttr paths: %v",
		unique, top, hot[start:]))
}

func (me *FileSystemDebug) AddTimingFileSystem(tfs *TimingFileSystem) {
	me.Add("timingfs-latencies",
		func() []byte { return FloatMapToBytes(tfs.Latencies()) })
	me.Add("timingfs-opcounts",
		func() []byte { return IntMapToBytes(tfs.OperationCounts()) })
	me.Add("timingfs-hotpaths",
		func() []byte { return hotPaths(tfs) })
}

func (me *FileSystemDebug) AddRawTimingFileSystem(tfs *TimingRawFileSystem) {
	me.Add("rawtimingfs-latencies",
		func() []byte { return FloatMapToBytes(tfs.Latencies()) })
}
