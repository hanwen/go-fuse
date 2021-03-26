package fs

import (
	"testing"
)

func TestInodeParents(t *testing.T) {
	var p inodeParents
	var ino1, ino2, ino3 Inode

	// empty store should be empty without panicing
	if count := p.count(); count != 0 {
		t.Error(count)
	}
	if p.all() != nil {
		t.Error("empty store should return nil but did not")
	}

	// non-dupes should be stored
	all := []parentData{
		parentData{"foo", &ino1},
		parentData{"foo2", &ino1},
		parentData{"foo3", &ino1},
		parentData{"foo", &ino2},
		parentData{"foo", &ino3},
	}
	for i, v := range all {
		p.add(v)
		if count := p.count(); count != i+1 {
			t.Errorf("want=%d have=%d", i+1, count)
		}
		last := p.get()
		if *last != v {
			t.Error("get did not give us last-known parent")
		}
	}

	// adding dupes should not cause the count to increase, but
	// must cause get() to return the most recently added dupe.
	for _, v := range all {
		p.add(v)
		if count := p.count(); count != len(all) {
			t.Errorf("want=%d have=%d", len(all), count)
		}
		last := p.get()
		if *last != v {
			t.Error("get did not give us last-known parent")
		}
	}

	all2 := p.all()
	if len(all) != len(all2) {
		t.Errorf("want=%d have=%d", len(all), len(all2))
	}
}
