// +build linux

package fuse

import (
	"io/ioutil"
	"os"
	"syscall"
	"testing"
)

func TestLinkAt(t *testing.T) {
	dir, _ := ioutil.TempDir("", "go-fuse-misc_test")
	ioutil.WriteFile(dir+"/a", []byte{42}, 0644)
	f, _ := os.Open(dir)
	e := Linkat(int(f.Fd()), "a", int(f.Fd()), "b")
	if e != 0 {
		t.Fatalf("Linkat %d", e)
	}

	var s1, s2 syscall.Stat_t
	err := syscall.Lstat(dir+"/a", &s1)
	if err != nil {
		t.Fatalf("Lstat a: %v", err)
	}
	err = syscall.Lstat(dir+"/b", &s2)
	if err != nil {
		t.Fatalf("Lstat b: %v", err)
	}

	if s1.Ino != s2.Ino {
		t.Fatal("Ino mismatch", s1, s2)
	}
}
