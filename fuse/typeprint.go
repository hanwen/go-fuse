package fuse

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var openFlagNames map[int]string
var initFlagNames map[int]string
var fuseOpenFlagNames map[int]string
var writeFlagNames map[int]string
var readFlagNames map[int]string
var releaseFlagNames map[int]string
var accessFlagName map[int]string

func init() {
	releaseFlagNames = map[int]string{
		RELEASE_FLUSH: "FLUSH",
	}
	openFlagNames = map[int]string{
		os.O_WRONLY:   "WRONLY",
		os.O_RDWR:     "RDWR",
		os.O_APPEND:   "APPEND",
		os.O_ASYNC:    "ASYNC",
		os.O_CREATE:   "CREAT",
		os.O_EXCL:     "EXCL",
		os.O_NOCTTY:   "NOCTTY",
		os.O_NONBLOCK: "NONBLOCK",
		os.O_SYNC:     "SYNC",
		os.O_TRUNC:    "TRUNC",

		syscall.O_CLOEXEC:   "CLOEXEC",
		syscall.O_DIRECT:    "DIRECT",
		syscall.O_DIRECTORY: "DIRECTORY",
		syscall.O_LARGEFILE: "LARGEFILE",
		syscall.O_NOATIME:   "NOATIME",
	}
	initFlagNames = map[int]string{
		CAP_ASYNC_READ:     "ASYNC_READ",
		CAP_POSIX_LOCKS:    "POSIX_LOCKS",
		CAP_FILE_OPS:       "FILE_OPS",
		CAP_ATOMIC_O_TRUNC: "ATOMIC_O_TRUNC",
		CAP_EXPORT_SUPPORT: "EXPORT_SUPPORT",
		CAP_BIG_WRITES:     "BIG_WRITES",
		CAP_DONT_MASK:      "DONT_MASK",
		CAP_SPLICE_WRITE:   "SPLICE_WRITE",
		CAP_SPLICE_MOVE:    "SPLICE_MOVE",
		CAP_SPLICE_READ:    "SPLICE_READ",
	}
	fuseOpenFlagNames = map[int]string{
		FOPEN_DIRECT_IO:   "DIRECT",
		FOPEN_KEEP_CACHE:  "CACHE",
		FOPEN_NONSEEKABLE: "NONSEEK",
	}
	writeFlagNames = map[int]string{
		WRITE_CACHE:     "CACHE",
		WRITE_LOCKOWNER: "LOCKOWNER",
	}
	readFlagNames = map[int]string{
		READ_LOCKOWNER: "LOCKOWNER",
	}
	accessFlagName = map[int]string{
		X_OK: "x",
		W_OK: "w",
		R_OK: "r",
	}
}

func flagString(names map[int]string, fl int, def string) string {
	s := []string{}
	for k, v := range names {
		if fl&k != 0 {
			s = append(s, v)
			fl ^= k
		}
	}
	if len(s) == 0 && def != "" {
		s = []string{def}
	}
	if fl != 0 {
		s = append(s, fmt.Sprintf("0x%x", fl))
	}

	return strings.Join(s, ",")
}

func (me *OpenIn) String() string {
	return fmt.Sprintf("{%s}", flagString(openFlagNames, int(me.Flags), "O_RDONLY"))
}

func (me *SetAttrIn) String() string {
	s := []string{}
	if me.Valid&FATTR_MODE != 0 {
		s = append(s, fmt.Sprintf("mode 0%o", me.Mode))
	}
	if me.Valid&FATTR_UID != 0 {
		s = append(s, fmt.Sprintf("uid %d", me.Uid))
	}
	if me.Valid&FATTR_GID != 0 {
		s = append(s, fmt.Sprintf("uid %d", me.Gid))
	}
	if me.Valid&FATTR_SIZE != 0 {
		s = append(s, fmt.Sprintf("size %d", me.Size))
	}
	if me.Valid&FATTR_ATIME != 0 {
		s = append(s, fmt.Sprintf("atime %d %d", me.Atime, me.Atimensec))
	}
	if me.Valid&FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("mtime %d %d", me.Mtime, me.Mtimensec))
	}
	if me.Valid&FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("fh %d", me.Fh))
	}
	// TODO - FATTR_ATIME_NOW = (1 << 7), FATTR_MTIME_NOW = (1 << 8), FATTR_LOCKOWNER = (1 << 9)
	return fmt.Sprintf("{%s}", strings.Join(s, ", "))
}

func (me *Attr) String() string {
	return fmt.Sprintf(
		"{M0%o S=%d L=%d "+
			"%d:%d "+
			"%d*%d %d:%d "+
			"A %d.%09d "+
			"M %d.%09d "+
			"C %d.%09d}",
		me.Mode, me.Size, me.Nlink,
		me.Uid, me.Gid,
		me.Blocks, me.Blksize,
		me.Rdev, me.Ino, me.Atime, me.Atimensec, me.Mtime, me.Mtimensec,
		me.Ctime, me.Ctimensec)
}

func (me *AttrOut) String() string {
	return fmt.Sprintf(
		"{A%d.%09d %v}",
		me.AttrValid, me.AttrValidNsec, &me.Attr)
}

func (me *CreateIn) String() string {
	return fmt.Sprintf(
		"{0%o [%s] (0%o)}", me.Mode,
		flagString(openFlagNames, int(me.Flags), "O_RDONLY"), me.Umask)
}

func (me *EntryOut) String() string {
	return fmt.Sprintf("{%d E%d.%09d A%d.%09d %v}",
		me.NodeId, me.EntryValid, me.EntryValidNsec,
		me.AttrValid, me.AttrValidNsec, &me.Attr)
}

func (me *CreateOut) String() string {
	return fmt.Sprintf("{%v %v}", &me.EntryOut, &me.OpenOut)
}

func (me *OpenOut) String() string {
	return fmt.Sprintf("{Fh %d %s}", me.Fh,
		flagString(fuseOpenFlagNames, int(me.OpenFlags), ""))
}

func (me *GetAttrIn) String() string {
	return fmt.Sprintf("{Fh %d}", me.Fh)
}

func (me *InitIn) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s}",
		me.Major, me.Minor, me.MaxReadAhead,
		flagString(initFlagNames, int(me.Flags), ""))
}
func (me *InitOut) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s %d/%d Wr 0x%x}",
		me.Major, me.Minor, me.MaxReadAhead,
		flagString(initFlagNames, int(me.Flags), ""),
		me.CongestionThreshold, me.MaxBackground, me.MaxWrite)
}

func (me *ReadIn) String() string {
	return fmt.Sprintf("{Fh %d off %d sz %d %s L %d %s}",
		me.Fh, me.Offset, me.Size,
		flagString(readFlagNames, int(me.ReadFlags), ""),
		me.LockOwner,
		flagString(openFlagNames, int(me.Flags), "RDONLY"))
}

func (me *MkdirIn) String() string {
	return fmt.Sprintf("{0%o (0%o)}", me.Mode, me.Umask)
}

func (me *MknodIn) String() string {
	return fmt.Sprintf("{0%o (0%o), %d}", me.Mode, me.Umask, me.Rdev)
}

func (me *ReleaseIn) String() string {
	return fmt.Sprintf("{Fh %d %s %s L%d}",
		me.Fh, flagString(openFlagNames, int(me.Flags), ""),
		flagString(releaseFlagNames, int(me.ReleaseFlags), ""),
		me.LockOwner)
}

func (me *FlushIn) String() string {
	return fmt.Sprintf("{Fh %d}", me.Fh)
}

func (me *AccessIn) String() string {
	return fmt.Sprintf("{%s}", flagString(accessFlagName, int(me.Mask), ""))
}

func (me *Kstatfs) String() string {
	return fmt.Sprintf(
		"{b%d f%d fs%d ff%d bs%d nl%d frs%d}",
		me.Blocks, me.Bfree, me.Bavail, me.Files, me.Ffree,
		me.Bsize, me.NameLen, me.Frsize)
}

func (me *WithFlags) String() string {
	return fmt.Sprintf("File %s (%s) %s %s",
		me.File, me.Description, flagString(openFlagNames, int(me.OpenFlags), "O_RDONLY"),
		flagString(fuseOpenFlagNames, int(me.FuseFlags), ""))
}
