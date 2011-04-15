package unionfs

import (
	"fmt"
	"log"
	"time"
	"testing"
)

var _ = fmt.Print
var _ = log.Print

func TestTimedIntCache(t *testing.T) {
	fetchCount := 0
	fetch := func(n string) interface{} {
		fetchCount++
		i := int(n[0])
		return &i
	}

	var ttl int64
	ttl = 1e6

	cache := NewTimedCache(fetch, ttl)
	v := cache.Get("n").(*int)
	if *v != int('n') {
		t.Error("value mismatch", v)
	}
	if fetchCount != 1 {
		t.Error("fetch count mismatch", fetchCount)
	}

	// The cache update is async.
	time.Sleep(ttl / 10)

	w := cache.Get("n")
	if v != w {
		t.Error("Huh, inconsistent.")
	}

	if fetchCount > 1 {
		t.Error("fetch count fail.", fetchCount)
	}

	time.Sleep(ttl * 2)
	cache.Purge()

	w = cache.Get("n")
	if fetchCount == 1 {
		t.Error("did not fetch again. Purge unsuccessful?")
	}
}
