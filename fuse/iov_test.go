package fuse

import (
	"reflect"
	"testing"
)

func TestIovCopy(t *testing.T) {
	src := [][]byte{
		[]byte("xyz"),
		nil,
		[]byte("pqr"),
	}

	dest := [][]byte{
		make([]byte, 3),
		make([]byte, 4),
	}

	n := iovCopy(dest, src)
	if n != 6 {
		t.Fatal("want 6 got ", n)
	}

	want := [][]byte{
		[]byte("xyz"),
		[]byte("pqr\000"),
	}
	if !reflect.DeepEqual(want, dest) {
		t.Errorf("got %#v want %#v", dest, want)
	}
}
