package unionfs

import (
	"log"
	"sync"
	"time"
)

var _ = log.Println

type cacheEntry struct {
	data interface{}

	// expiry is the absolute timestamp of the expiry.
	expiry time.Time
}

// TimedIntCache caches the result of fetch() for some time.  It is
// thread-safe.  Calls of fetch() do no happen inside a critical
// section, so when multiple concurrent Get()s happen for the same
// key, multiple fetch() calls may be issued for the same key.
type TimedCacheFetcher func(name string) (value interface{}, cacheable bool)
type TimedCache struct {
	fetch TimedCacheFetcher

	// ttl is the duration of the cache.
	ttl time.Duration

	cacheMapMutex sync.RWMutex
	cacheMap      map[string]*cacheEntry

	PurgeTimer *time.Timer
}

// Creates a new cache with the given TTL.  If TTL <= 0, the caching is
// indefinite.
func NewTimedCache(fetcher TimedCacheFetcher, ttl time.Duration) *TimedCache {
	l := new(TimedCache)
	l.ttl = ttl
	l.fetch = fetcher
	l.cacheMap = make(map[string]*cacheEntry)
	return l
}

func (me *TimedCache) Get(name string) interface{} {
	me.cacheMapMutex.RLock()
	info, ok := me.cacheMap[name]
	me.cacheMapMutex.RUnlock()

	valid := ok && (me.ttl <= 0 || info.expiry.After(time.Now()))
	if valid {
		return info.data
	}
	return me.GetFresh(name)
}

func (me *TimedCache) Set(name string, val interface{}) {
	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()

	me.cacheMap[name] = &cacheEntry{
		data:   val,
		expiry: time.Now().Add(me.ttl),
	}
}

func (me *TimedCache) DropEntry(name string) {
	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()

	delete(me.cacheMap, name)
}

func (me *TimedCache) GetFresh(name string) interface{} {
	data, ok := me.fetch(name)
	if ok {
		me.Set(name, data)
	}
	return data
}

// Drop all expired entries.
func (me *TimedCache) Purge() {
	keys := make([]string, 0, len(me.cacheMap))
	now := time.Now()

	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()
	for k, v := range me.cacheMap {
		if now.After(v.expiry) {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		delete(me.cacheMap, k)
	}
}

func (me *TimedCache) RecurringPurge() {
	if me.ttl <= 0 {
		return
	}

	me.Purge()
	me.PurgeTimer = time.AfterFunc(int64(me.ttl*5),
		func() { me.RecurringPurge() })
}

func (me *TimedCache) DropAll(names []string) {
	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()

	if names == nil {
		me.cacheMap = make(map[string]*cacheEntry, len(me.cacheMap))
	} else {
		for _, nm := range names {
			delete(me.cacheMap, nm)
		}
	}
}
