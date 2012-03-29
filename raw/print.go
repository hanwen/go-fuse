package raw
import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var initFlagNames map[int]string
var releaseFlagNames map[int]string
var OpenFlagNames map[int]string
var FuseOpenFlagNames map[int]string

func init() {
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
	releaseFlagNames = map[int]string{
		RELEASE_FLUSH: "FLUSH",
	}
	OpenFlagNames = map[int]string{
		os.O_WRONLY:        "WRONLY",
		os.O_RDWR:          "RDWR",
		os.O_APPEND:        "APPEND",
		syscall.O_ASYNC:    "ASYNC",
		os.O_CREATE:        "CREAT",
		os.O_EXCL:          "EXCL",
		syscall.O_NOCTTY:   "NOCTTY",
		syscall.O_NONBLOCK: "NONBLOCK",
		os.O_SYNC:          "SYNC",
		os.O_TRUNC:         "TRUNC",

		syscall.O_CLOEXEC:   "CLOEXEC",
		syscall.O_DIRECT:    "DIRECT",
		syscall.O_DIRECTORY: "DIRECTORY",
		syscall.O_LARGEFILE: "LARGEFILE",
		syscall.O_NOATIME:   "NOATIME",
	}
	FuseOpenFlagNames = map[int]string{
		FOPEN_DIRECT_IO:   "DIRECT",
		FOPEN_KEEP_CACHE:  "CACHE",
		FOPEN_NONSEEKABLE: "NONSEEK",
	}
}

func FlagString(names map[int]string, fl int, def string) string {
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
	
func (me *ForgetIn) String() string {
	return fmt.Sprintf("{%d}", me.Nlookup)
}

func (me *BatchForgetIn) String() string {
	return fmt.Sprintf("{%d}", me.Count)
}


func (me *MkdirIn) String() string {
	return fmt.Sprintf("{0%o (0%o)}", me.Mode, me.Umask)
}

func (me *MknodIn) String() string {
	return fmt.Sprintf("{0%o (0%o), %d}", me.Mode, me.Umask, me.Rdev)
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
		s = append(s, fmt.Sprintf("atime %d.%09d", me.Atime, me.Atimensec))
	}
	if me.Valid&FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("mtime %d.%09d", me.Mtime, me.Mtimensec))
	}
	if me.Valid&FATTR_MTIME != 0 {
		s = append(s, fmt.Sprintf("fh %d", me.Fh))
	}
	// TODO - FATTR_ATIME_NOW = (1 << 7), FATTR_MTIME_NOW = (1 << 8), FATTR_LOCKOWNER = (1 << 9)
	return fmt.Sprintf("{%s}", strings.Join(s, ", "))
}

func (me *GetAttrIn) String() string {
	return fmt.Sprintf("{Fh %d}", me.Fh)
}


func (me *ReleaseIn) String() string {
	return fmt.Sprintf("{Fh %d %s %s L%d}",
		me.Fh, FlagString(OpenFlagNames, int(me.Flags), ""),
		FlagString(releaseFlagNames, int(me.ReleaseFlags), ""),
		me.LockOwner)
}

func (me *OpenIn) String() string {
	return fmt.Sprintf("{%s}", FlagString(OpenFlagNames, int(me.Flags), "O_RDONLY"))
}

func (me *OpenOut) String() string {
	return fmt.Sprintf("{Fh %d %s}", me.Fh,
		FlagString(FuseOpenFlagNames, int(me.OpenFlags), ""))
}

func (me *InitIn) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s}",
		me.Major, me.Minor, me.MaxReadAhead,
		FlagString(initFlagNames, int(me.Flags), ""))
}

func (me *InitOut) String() string {
	return fmt.Sprintf("{%d.%d Ra 0x%x %s %d/%d Wr 0x%x}",
		me.Major, me.Minor, me.MaxReadAhead,
		FlagString(initFlagNames, int(me.Flags), ""),
		me.CongestionThreshold, me.MaxBackground, me.MaxWrite)
}

func (me *SetXAttrIn) String() string {
	return fmt.Sprintf("{sz %d f%o}", me.Size, me.Flags)
}

func (me *GetXAttrIn) String() string {
	return fmt.Sprintf("{sz %d}", me.Size)
}

func (me *GetXAttrOut) String() string {
	return fmt.Sprintf("{sz %d}", me.Size)
}
