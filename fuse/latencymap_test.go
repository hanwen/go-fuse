package fuse

import (
	"fmt"
	"testing"
)

var _ = fmt.Println

func TestLatencyMap(t *testing.T) {
	m := NewLatencyMap()
	m.Add("foo", "", 0.1e9)
	m.Add("foo", "", 0.2e9)

	l := m.Latencies(1e-3)
	if l["foo"] != 150 {
		t.Error("unexpected latency", l)
	}
}
