package fs

import (
	"syscall"
	"testing"
)

func TestInodeIsDir(t *testing.T) {
	cases := []struct {
		mode uint32
		dir  bool
	}{
		{syscall.S_IFBLK, false},
		{syscall.S_IFCHR, false},
		{syscall.S_IFDIR, true},
		{syscall.S_IFIFO, false},
		{syscall.S_IFLNK, false},
		{syscall.S_IFREG, false},
		{syscall.S_IFSOCK, false},
	}
	var i Inode
	for _, c := range cases {
		i.stableAttr.Mode = c.mode
		if i.IsDir() != c.dir {
			t.Errorf("wrong result for case %#v", c)
		}
	}
}
