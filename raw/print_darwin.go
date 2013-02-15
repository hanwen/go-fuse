package raw
import (
	"fmt"
	"syscall"
)

func (a *Attr) String() string {
	return fmt.Sprintf(
		"{M0%o SZ=%d L=%d "+
			"%d:%d "+
			"%d %d:%d "+
			"A %d.%09d "+
			"M %d.%09d "+
			"C %d.%09d}",
		a.Mode, a.Size, a.Nlink,
		a.Uid, a.Gid,
		a.Blocks,
		a.Rdev, a.Ino, a.Atime, a.Atimensec, a.Mtime, a.Mtimensec,
		a.Ctime, a.Ctimensec)
}
