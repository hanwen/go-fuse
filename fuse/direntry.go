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
	offset uint64
}

func NewDirEntryList(data []byte, off uint64) *DirEntryList {
	return &DirEntryList{
		buf:    data[:0],
		offset: off,
	}
}

func (l *DirEntryList) AddDirEntry(e DirEntry) bool {
	return l.Add(e.Name, uint64(raw.FUSE_UNKNOWN_INO), e.Mode)
}

func (l *DirEntryList) Add(name string, inode uint64, mode uint32) bool {
	padding := (8 - len(name)&7) & 7
	delta := padding + direntSize + len(name)
	oldLen := len(l.buf)
	newLen := delta + oldLen

	if newLen > cap(l.buf) {
		return false
	}
	l.buf = l.buf[:newLen]
	dirent := (*raw.Dirent)(unsafe.Pointer(&l.buf[oldLen]))
	dirent.Off = l.offset + 1
	dirent.Ino = inode
	dirent.NameLen = uint32(len(name))
	dirent.Typ = ModeToType(mode)
	oldLen += direntSize
	copy(l.buf[oldLen:], name)
	oldLen += len(name)

	if padding > 0 {
		copy(l.buf[oldLen:], eightPadding[:padding])
	}

	l.offset = dirent.Off
	return true
}

func (l *DirEntryList) Bytes() []byte {
	return l.buf
}

////////////////////////////////////////////////////////////////

type rawDir interface {
	ReadDir(out *DirEntryList, input *ReadIn) Status
	Release()
}

type connectorDir struct {
	node       FsNode
	stream     []DirEntry
	lastOffset uint64
}

func (d *connectorDir) ReadDir(list *DirEntryList, input *ReadIn) (code Status) {
	if d.stream == nil {
		return OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir(nil)
		if !code.Ok() {
			return code
		}
	}

	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if !list.AddDirEntry(e) {
			break
		}
	}
	d.lastOffset = list.offset
	return OK
}

// Read everything so we make goroutines exit.
func (d *connectorDir) Release() {
}
