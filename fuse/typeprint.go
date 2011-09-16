package fuse
import (
	"fmt"
	"os"
	"strings"
)

var openFlags map[int]string
func init() {
	openFlags = map[int]string{
		os.O_WRONLY   : "O_WRONLY",
		os.O_RDWR     : "O_RDWR",
		os.O_APPEND   : "O_APPEND",
		os.O_ASYNC    : "O_ASYNC",
		os.O_CREATE   : "O_CREAT",
		os.O_EXCL     : "O_EXCL",
		os.O_NOCTTY   : "O_NOCTTY",
		os.O_NONBLOCK : "O_NONBLOCK",
		os.O_SYNC     : "O_SYNC",
		os.O_TRUNC    : "O_TRUNC",
	}
}

func (me *OpenIn) String() string {
	s := []string{}
	for k, v := range openFlags {
		if me.Flags & uint32(k) != 0 {
			s = append(s, v)
		}
	}
	if len(s) == 0 {
		s = []string{"O_RDONLY"}
	}

	return fmt.Sprintf("[%s]", strings.Join(s, " | "))
}

func (me *SetAttrIn) String() string {
	s := []string{}
	if me.Valid & FATTR_MODE != 0 {
		s = append(s, fmt.Sprintf("mode %o", me.Mode))
	}
	if me.Valid & FATTR_UID != 0 {
		s = append(s, fmt.Sprintf("uid %d", me.Uid))
	}
	if me.Valid & FATTR_GID != 0 {
		s = append(s, fmt.Sprintf("uid %d", me.Gid))
	}
	if me.Valid & FATTR_SIZE != 0 {
		s = append(s, fmt.Sprintf("uid %d", me.Size))
	}
	if me.Valid & FATTR_ATIME != 0 {
		s = append(s, fmt.Sprintf("atime %d %d", me.Atime, me.Atimensec))
	}
	if me.Valid & FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("mtime %d %d", me.Mtime, me.Mtimensec))
	}
	if me.Valid & FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("fh %d", me.Fh))
	}
	// TODO - FATTR_ATIME_NOW = (1 << 7), FATTR_MTIME_NOW = (1 << 8), FATTR_LOCKOWNER = (1 << 9)
	return fmt.Sprintf("[%s]", strings.Join(s, ", "))
}
