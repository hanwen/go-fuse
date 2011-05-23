package unionfs

import (
	"github.com/hanwen/go-fuse/fuse"
	"sync"
	"log"
	"time"
)

// newDirnameMap reads the contents of the given directory. On error,
// returns a nil map. This forces reloads in the DirCache until we
// succeed.
func newDirnameMap(fs fuse.FileSystem, dir string) map[string]bool {
	stream, code := fs.OpenDir(dir)
	if !code.Ok() {
		log.Printf("newDirnameMap(): %v %v", dir, code)
		return nil
	}
	
	result := make(map[string]bool)
	for e := range stream {
		if e.Mode&fuse.S_IFREG != 0 {
			result[e.Name] = true
		}
	}
	return result
}

// DirCache caches names in a directory for some time.
//
// If called when the cache is expired, the filenames are read afresh in
// the background.
type DirCache struct {
	dir   string
	ttlNs int64
	fs    fuse.FileSystem
	// Protects data below.
	lock sync.RWMutex

	// If nil, you may call refresh() to schedule a new one.
	names         map[string]bool
	updateRunning bool
}

func (me *DirCache) setMap(newMap map[string]bool) {
	me.lock.Lock()
	defer me.lock.Unlock()

	me.names = newMap
	me.updateRunning = false
	_ = time.AfterFunc(me.ttlNs,
		func() { me.DropCache() })
}

func (me *DirCache) DropCache() {
	me.lock.Lock()
	defer me.lock.Unlock()
	me.names = nil
}

// Try to refresh: if another update is already running, do nothing,
// otherwise, read the directory and set it.
func (me *DirCache) maybeRefresh() {
	me.lock.Lock()
	defer me.lock.Unlock()
	if me.updateRunning {
		return
	}
	me.updateRunning = true
	go func() {
		newmap := newDirnameMap(me.fs, me.dir)
		me.setMap(newmap)
	}()
}

func (me *DirCache) RemoveEntry(name string) {
	me.lock.Lock()
	defer me.lock.Unlock()
	if me.names == nil {
		go me.maybeRefresh()
		return
	}

	me.names[name] = false, false
}

func (me *DirCache) AddEntry(name string) {
	me.lock.Lock()
	defer me.lock.Unlock()
	if me.names == nil {
		go me.maybeRefresh()
		return
	}

	me.names[name] = true
}

func NewDirCache(fs fuse.FileSystem, dir string, ttlNs int64) *DirCache {
	dc := new(DirCache)
	dc.dir = dir
	dc.fs = fs
	dc.ttlNs = ttlNs
	return dc
}

func (me *DirCache) HasEntry(name string) (mapPresent bool, found bool) {
	me.lock.RLock()
	defer me.lock.RUnlock()

	if me.names == nil {
		go me.maybeRefresh()
		return false, false
	}

	return true, me.names[name]
}
