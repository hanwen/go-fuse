package fuse

import (
	"log"
	"os"
	"syscall"
	"time"
)

type FileMode uint32

func (me FileMode) String() string {
	switch uint32(me) & syscall.S_IFMT {
	case syscall.S_IFIFO:
		return "p"
	case syscall.S_IFCHR:
		return "c"
	case syscall.S_IFDIR:
		return "d"
	case syscall.S_IFBLK:
		return "b"
	case syscall.S_IFREG:
		return "f"
	case syscall.S_IFLNK:
		return "l"
	case syscall.S_IFSOCK:
		return "s"
	default:
		log.Panic("Unknown mode: %o", me)
	}
	return "0"
}

func (me FileMode) IsFifo() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFIFO }

// IsChar reports whether the FileInfo describes a character special file.
func (me FileMode) IsChar() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFCHR }

// IsDir reports whether the FileInfo describes a directory.
func (me FileMode) IsDir() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFDIR }

// IsBlock reports whether the FileInfo describes a block special file.
func (me FileMode) IsBlock() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFBLK }

// IsRegular reports whether the FileInfo describes a regular file.
func (me FileMode) IsRegular() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFREG }

// IsSymlink reports whether the FileInfo describes a symbolic link.
func (me FileMode) IsSymlink() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFLNK }

// IsSocket reports whether the FileInfo describes a socket.
func (me FileMode) IsSocket() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFSOCK }

func (me *Attr) IsFifo() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFIFO }

// IsChar reports whether the FileInfo describes a character special file.
func (me *Attr) IsChar() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFCHR }

// IsDir reports whether the FileInfo describes a directory.
func (me *Attr) IsDir() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFDIR }

// IsBlock reports whether the FileInfo describes a block special file.
func (me *Attr) IsBlock() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFBLK }

// IsRegular reports whether the FileInfo describes a regular file.
func (me *Attr) IsRegular() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFREG }

// IsSymlink reports whether the FileInfo describes a symbolic link.
func (me *Attr) IsSymlink() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFLNK }

// IsSocket reports whether the FileInfo describes a socket.
func (me *Attr) IsSocket() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFSOCK }

func (a *Attr) Atimens() int64 {
	return int64(1e9*a.Atime) + int64(a.Atimensec)
}

func (a *Attr) Mtimens() int64 {
	return int64(1e9*a.Mtime) + int64(a.Mtimensec)
}

func (a *Attr) Ctimens() int64 {
	return int64(1e9*a.Ctime) + int64(a.Ctimensec)
}

func (a *Attr) SetNs(atimens int64, mtimens int64, ctimens int64) {
	if atimens >= 0 {
		a.Atime = uint64(atimens / 1e9)
		a.Atimensec = uint32(atimens % 1e9)
	}
	if mtimens >= 0 {
		a.Mtime = uint64(mtimens / 1e9)
		a.Mtimensec = uint32(mtimens % 1e9)
	}
	if atimens >= 0 {
		a.Ctime = uint64(ctimens / 1e9)
		a.Ctimensec = uint32(ctimens % 1e9)
	}
}

func (a *Attr) SetTimes(access *time.Time, mod *time.Time, chstatus *time.Time) {
	if access != nil {
		atimens := access.UnixNano()
		a.Atime = uint64(atimens / 1e9)
		a.Atimensec = uint32(atimens % 1e9)
	}
	if mod != nil {
		mtimens := mod.UnixNano()
		a.Mtime = uint64(mtimens / 1e9)
		a.Mtimensec = uint32(mtimens % 1e9)
	}
	if chstatus != nil {
		ctimens := chstatus.UnixNano()
		a.Ctime = uint64(ctimens / 1e9)
		a.Ctimensec = uint32(ctimens % 1e9)
	}
}

func (a *Attr) FromStat(s *syscall.Stat_t) {
	a.Ino = uint64(s.Ino)
	a.Size = uint64(s.Size)
	a.Blocks = uint64(s.Blocks)
	a.Atime = uint64(s.Atim.Sec)
	a.Atimensec = uint32(s.Atim.Nsec)
	a.Mtime = uint64(s.Mtim.Sec)
	a.Mtimensec = uint32(s.Mtim.Nsec)
	a.Ctime = uint64(s.Ctim.Sec)
	a.Ctimensec = uint32(s.Ctim.Nsec)
	a.Mode = s.Mode
	a.Nlink = uint32(s.Nlink)
	a.Uid = uint32(s.Uid)
	a.Gid = uint32(s.Gid)
	a.Rdev = uint32(s.Rdev)
	a.Blksize = uint32(s.Blksize)
}

func (a *Attr) FromFileInfo(fi os.FileInfo) {
	stat := fi.(*os.FileStat)
	sys := stat.Sys.(*syscall.Stat_t)
	a.FromStat(sys)
}

func (a *Attr) ChangeTime() time.Time {
	return time.Unix(int64(a.Ctime), int64(a.Ctimensec))
}

func (a *Attr) AccessTime() time.Time {
	return time.Unix(int64(a.Atime), int64(a.Atimensec))
}

func (a *Attr) ModTime() time.Time {
	return time.Unix(int64(a.Mtime), int64(a.Mtimensec))
}

func ToStatT(f os.FileInfo) *syscall.Stat_t {
	return f.(*os.FileStat).Sys.(*syscall.Stat_t)
}

func ToAttr(f os.FileInfo) *Attr {
	if f == nil {
		return nil
	}
	a := &Attr{}
	a.FromStat(ToStatT(f))
	return a
}
