package fuse

// all of the code for DirEntryList.

import (
	"log"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Print
var eightPadding [8]byte

const direntSize = int(unsafe.Sizeof(raw.Dirent{}))

// DirEntry is a type for PathFileSystem and NodeFileSystem to return
// directory contents in.
type DirEntry struct {
	Mode uint32
	Name string
}

type DirEntryList struct {
	buf    []byte
	size   int

	// TODO - hide this again.
	Offset uint64
}

func NewDirEntryList(data []byte, off uint64) *DirEntryList {
	return &DirEntryList{
		buf:    data[:0],
		size:   len(data),
		Offset: off,
	}
}

// AddDirEntry tries to add an entry.
func (l *DirEntryList) AddDirEntry(e DirEntry) bool {
	return l.Add(e.Name, uint64(raw.FUSE_UNKNOWN_INO), e.Mode)
}

func (l *DirEntryList) Add(name string, inode uint64, mode uint32) bool {
	padding := (8 - len(name)&7) & 7
	delta := padding + direntSize + len(name)
	oldLen := len(l.buf)
	newLen := delta + oldLen

	if newLen > l.size {
		return false
	}
	l.buf = l.buf[:newLen]
	dirent := (*raw.Dirent)(unsafe.Pointer(&l.buf[oldLen]))
	dirent.Off = l.Offset + 1
	dirent.Ino = inode
	dirent.NameLen = uint32(len(name))
	dirent.Typ = ModeToType(mode)
	oldLen += direntSize
	copy(l.buf[oldLen:], name)
	oldLen += len(name)

	if padding > 0 {
		copy(l.buf[oldLen:], eightPadding[:padding])
	}

	l.Offset = dirent.Off
	return true
}

func (l *DirEntryList) Bytes() []byte {
	return l.buf
}

////////////////////////////////////////////////////////////////


