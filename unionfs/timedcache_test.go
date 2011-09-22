package unionfs

import (
	"fmt"
	"log"
	"time"
	"testing"
)

var _ = fmt.Print
var _ = log.Print

func TestTimedCache(t *testing.T) {
	fetchCount := 0
	fetch := func(n string) interface{} {
		fetchCount++
		i := int(n[0])
		return &i
	}

	var ttl int64

	// This fails with 1e6 on some Opteron CPUs.
	ttl = 1e8

	cache := NewTimedCache(fetch, ttl)
	v := cache.Get("n").(*int)
	if *v != int('n') {
		t.Errorf("value mismatch: got %d, want %d", *v, int('n'))
	}
	if fetchCount != 1 {
		t.Errorf("fetch count mismatch: got %d want 1", fetchCount)
	}

	// The cache update is async.
	time.Sleep(ttl / 10)

	w := cache.Get("n")
	if v != w {
		t.Errorf("Huh, inconsistent: 1st = %v != 2nd = %v", v, w)
	}

	if fetchCount > 1 {
		t.Errorf("fetch count fail: %d > 1", fetchCount)
	}

	time.Sleep(ttl * 2)
	cache.Purge()

	w = cache.Get("n")
	if fetchCount == 1 {
		t.Error("Did not fetch again. Purge unsuccessful?")
	}
}
