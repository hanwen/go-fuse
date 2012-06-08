package fuse

import (
	"sync"
)

type latencyMapEntry struct {
	count int
	ns    int64
}

type LatencyMap struct {
	sync.Mutex
	stats map[string]*latencyMapEntry
}

func NewLatencyMap() *LatencyMap {
	m := &LatencyMap{}
	m.stats = make(map[string]*latencyMapEntry)
	return m
}

func (m *LatencyMap) Get(name string) (count int, dtNs int64) {
	m.Mutex.Lock()
	l := m.stats[name]
	m.Mutex.Unlock()
	return l.count, l.ns
}

func (m *LatencyMap) Add(name string, dtNs int64) {
	m.Mutex.Lock()
	m.add(name, dtNs)
	m.Mutex.Unlock()
}

func (m *LatencyMap) add(name string, dtNs int64) {
	e := m.stats[name]
	if e == nil {
		e = new(latencyMapEntry)
		m.stats[name] = e
	}

	e.count++
	e.ns += dtNs
}

func (m *LatencyMap) Counts() map[string]int {
	r := make(map[string]int)
	m.Mutex.Lock()
	for k, v := range m.stats {
		r[k] = v.count
	}
	m.Mutex.Unlock()

	return r
}

// Latencies returns a map. Use 1e-3 for unit to get ms
// results.
func (m *LatencyMap) Latencies(unit float64) map[string]float64 {
	r := make(map[string]float64)
	m.Mutex.Lock()
	mult := 1 / (1e9 * unit)
	for key, ent := range m.stats {
		lat := mult * float64(ent.ns) / float64(ent.count)
		r[key] = lat
	}
	m.Mutex.Unlock()

	return r
}
