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

func (m *LatencyMap) AddMany(args []LatencyArg) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()
	for _, v := range args {
		m.add(v.Name, v.Arg, v.DtNs)
	}
}
func (m *LatencyMap) Add(name string, arg string, dtNs int64) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()
	m.add(name, arg, dtNs)
}

func (m *LatencyMap) add(name string, arg string, dtNs int64) {
	e := m.stats[name]
	if e == nil {
		e = new(latencyMapEntry)
		m.stats[name] = e
	}

	e.count++
	e.ns += dtNs
	if arg != "" {
		_, ok := m.secondaryStats[name]
		if !ok {
			m.secondaryStats[name] = make(map[string]int64)
		}
		// TODO - do something with secondaryStats[name]
	}
}

func (m *LatencyMap) Counts() map[string]int {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	r := make(map[string]int)
	for k, v := range m.stats {
		r[k] = v.count
	}
	return r
}

// Latencies returns a map. Use 1e-3 for unit to get ms
// results.
func (m *LatencyMap) Latencies(unit float64) map[string]float64 {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	r := make(map[string]float64)
	mult := 1 / (1e9 * unit)
	for key, ent := range m.stats {
		lat := mult * float64(ent.ns) / float64(ent.count)
		r[key] = lat
	}
	return r
}

func (m *LatencyMap) TopArgs(name string) []string {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	counts := m.secondaryStats[name]
	results := make([]string, 0, len(counts))
	for k, v := range counts {
		results = append(results, fmt.Sprintf("% 9d %s", v, k))
	}
	sort.Strings(results)
	return results
}
