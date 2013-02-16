package fuse

import (
	"io/ioutil"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestTouch(t *testing.T) {
	ts := NewTestCase(t)
	defer ts.Cleanup()

	contents := []byte{1, 2, 3}
	err := ioutil.WriteFile(ts.origFile, []byte(contents), 0700)
	CheckSuccess(err)
	err = os.Chtimes(ts.mountFile, time.Unix(42, 0), time.Unix(43, 0))
	CheckSuccess(err)

	var stat syscall.Stat_t
	err = syscall.Lstat(ts.mountFile, &stat)
	CheckSuccess(err)
	if stat.Atim.Sec != 42 || stat.Mtim.Sec != 43 {
		t.Errorf("Got wrong timestamps %v", stat)
	}
}

func clearStatfs(s *syscall.Statfs_t) {
	empty := syscall.Statfs_t{}
	s.Type = 0
	s.Fsid = empty.Fsid
	s.Spare = empty.Spare
	// TODO - figure out what this is for.
	s.Flags = 0
}
