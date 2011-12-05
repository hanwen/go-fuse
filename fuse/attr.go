package fuse

import (
	"log"
	"os"
	"syscall"
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

// IsDirectory reports whether the FileInfo describes a directory.
func (me FileMode) IsDirectory() bool { return (uint32(me) & syscall.S_IFMT) == syscall.S_IFDIR }

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

// IsDirectory reports whether the FileInfo describes a directory.
func (me *Attr) IsDirectory() bool { return (uint32(me.Mode) & syscall.S_IFMT) == syscall.S_IFDIR }

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

func (a *Attr) SetTimes(atimens int64, mtimens int64, ctimens int64) {
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

func (attr *Attr) FromFileInfo(fi *os.FileInfo) {
	attr.Ino = uint64(fi.Ino)
	attr.Size = uint64(fi.Size)
	attr.Blocks = uint64(fi.Blocks)
	attr.SetTimes(fi.Atime_ns, fi.Mtime_ns, fi.Ctime_ns)
	attr.Mode = fi.Mode
	attr.Nlink = uint32(fi.Nlink)
	attr.Uid = uint32(fi.Uid)
	attr.Gid = uint32(fi.Gid)
	attr.Rdev = uint32(fi.Rdev)
	attr.Blksize = uint32(fi.Blksize)
}
func (a *Attr) ToFileInfo() (fi *os.FileInfo) {
	return &os.FileInfo{
		Ino:      a.Ino,
		Size:     int64(a.Size),
		Atime_ns: a.Atimens(),
		Mtime_ns: a.Mtimens(),
		Ctime_ns: a.Ctimens(),
		Blocks:   int64(a.Blocks),
		Mode:     a.Mode,
		Nlink:    uint64(a.Nlink),
		Uid:      int(a.Uid),
		Gid:      int(a.Gid),
		Rdev:     uint64(a.Rdev),
		Blksize:  int64(a.Blksize),
	}
}
