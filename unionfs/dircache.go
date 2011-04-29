package unionfs

import (
	"os"
	"sync"
	"log"
	"time"
)


/*
 On error, returns an empty map, since we have little options
 for outputting any other diagnostics.
*/
func newDirnameMap(dir string) map[string]bool {
	result := make(map[string]bool)

	f, err := os.Open(dir)
	if err != nil {
		log.Printf("newDirnameMap(): %v %v", dir, err)
		return result
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		log.Printf("newDirnameMap(): readdirnames %v %v", dir, err)
		return result
	}

	for _, n := range names {
		result[n] = true
	}
	return result
}

/*
 Caches names in a directory for some time.

 If called when the cache is expired, the filenames are read afresh in
 the background.
*/
type DirCache struct {
	dir   string
	ttlNs int64

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
	me.names = nil
	me.lock.Unlock()
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
		me.setMap(newDirnameMap(me.dir))
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

func NewDirCache(dir string, ttlNs int64) *DirCache {
	dc := new(DirCache)
	dc.dir = dir
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
