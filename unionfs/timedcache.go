package unionfs

import (
	"log"
	"sync"
	"time"
)

var _ = log.Println

type cacheEntry struct {
	data interface{}

	// expiryNs is the absolute timestamp of the expiry.
	expiryNs int64
}

// TimedIntCache caches the result of fetch() for some time.
//
// Oh, how I wish we had generics.
type TimedCache struct {
	fetch func(name string) interface{}

	// ttlNs is a duration of the cache.
	ttlNs int64

	cacheMapMutex sync.RWMutex
	cacheMap      map[string]*cacheEntry

	PurgeTimer *time.Timer
}

const layerCacheTimeoutNs = 1e9

func NewTimedCache(fetcher func(name string) interface{}, ttlNs int64) *TimedCache {
	l := new(TimedCache)
	l.ttlNs = ttlNs
	l.fetch = fetcher
	l.cacheMap = make(map[string]*cacheEntry)
	return l
}

func (me *TimedCache) Get(name string) interface{} {
	me.cacheMapMutex.RLock()
	info, ok := me.cacheMap[name]
	me.cacheMapMutex.RUnlock()

	now := time.Nanoseconds()
	if ok && info.expiryNs > now {
		return info.data
	}
	return me.getDataNoCache(name)
}

func (me *TimedCache) Set(name string, val interface{}) {
	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()

	me.cacheMap[name] = &cacheEntry{
		data:     val,
		expiryNs: time.Nanoseconds() + me.ttlNs,
	}
}

func (me *TimedCache) DropEntry(name string) {
	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()

	me.cacheMap[name] = nil, false
}

func (me *TimedCache) getDataNoCache(name string) interface{} {
	data := me.fetch(name)
	me.Set(name, data)
	return data
}

// Drop all expired entries.
func (me *TimedCache) Purge() {
	keys := make([]string, 0, len(me.cacheMap))
	now := time.Nanoseconds()

	me.cacheMapMutex.Lock()
	defer me.cacheMapMutex.Unlock()
	for k, v := range me.cacheMap {
		if v.expiryNs < now {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		me.cacheMap[k] = nil, false
	}
}

func (me *TimedCache) RecurringPurge() {
	me.Purge()
	me.PurgeTimer = time.AfterFunc(5*me.ttlNs,
		func() { me.RecurringPurge() })
}
