package fuse

import (
	"fmt"
	"sort"
	"sync"
)

type latencyMapEntry struct {
	count int
	ns    int64
}

type LatencyArg struct {
	Name string
	Arg  string
	DtNs int64
}

type LatencyMap struct {
	sync.Mutex
	stats          map[string]*latencyMapEntry
	secondaryStats map[string]map[string]int64
}

func NewLatencyMap() *LatencyMap {
	m := &LatencyMap{}
	m.stats = make(map[string]*latencyMapEntry)
	m.secondaryStats = make(map[string]map[string]int64)
	return m
}

func (me *LatencyMap) AddMany(args []LatencyArg) {
	me.Mutex.Lock()
	defer me.Mutex.Unlock()
	for _, v := range args {
		me.add(v.Name, v.Arg, v.DtNs)
	}
}
func (me *LatencyMap) Add(name string, arg string, dtNs int64) {
	me.Mutex.Lock()
	defer me.Mutex.Unlock()
	me.add(name, arg, dtNs)
}

func (me *LatencyMap) add(name string, arg string, dtNs int64) {
	e := me.stats[name]
	if e == nil {
		e = new(latencyMapEntry)
		me.stats[name] = e
	}

	e.count++
	e.ns += dtNs
	if arg != "" {
		m, ok := me.secondaryStats[name]
		if !ok {
			m = make(map[string]int64)
			me.secondaryStats[name] = m
		}
	}
}

func (me *LatencyMap) Counts() map[string]int {
	me.Mutex.Lock()
	defer me.Mutex.Unlock()

	r := make(map[string]int)
	for k, v := range me.stats {
		r[k] = v.count
	}
	return r
}

// Latencies returns a map. Use 1e-3 for unit to get ms
// results.
func (me *LatencyMap) Latencies(unit float64) map[string]float64 {
	me.Mutex.Lock()
	defer me.Mutex.Unlock()

	r := make(map[string]float64)
	mult := 1 / (1e9 * unit)
	for key, ent := range me.stats {
		lat := mult * float64(ent.ns) / float64(ent.count)
		r[key] = lat
	}
	return r
}

func (me *LatencyMap) TopArgs(name string) []string {
	me.Mutex.Lock()
	defer me.Mutex.Unlock()

	counts := me.secondaryStats[name]
	results := make([]string, 0, len(counts))
	for k, v := range counts {
		results = append(results, fmt.Sprintf("% 9d %s", v, k))
	}
	sort.Strings(results)
	return results
}
