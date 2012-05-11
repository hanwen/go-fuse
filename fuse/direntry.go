package fuse

// all of the code for DirEntryList.

import (
	"bytes"
	"log"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Print

// DirEntry is a type for PathFileSystem and NodeFileSystem to return
// directory contents in.
type DirEntry struct {
	Mode uint32
	Name string
}

type DirEntryList struct {
	buf     bytes.Buffer
	offset  uint64
	maxSize int
}

func NewDirEntryList(max int, off uint64) *DirEntryList {
	return &DirEntryList{maxSize: max, offset: off}
}

func (l *DirEntryList) AddString(name string, inode uint64, mode uint32) bool {
	return l.Add([]byte(name), inode, mode)
}

func (l *DirEntryList) AddDirEntry(e DirEntry) bool {
	return l.Add([]byte(e.Name), uint64(raw.FUSE_UNKNOWN_INO), e.Mode)
}

func (l *DirEntryList) Add(name []byte, inode uint64, mode uint32) bool {
	lastLen := l.buf.Len()

	l.offset++
	dirent := raw.Dirent{
		Off:     l.offset,
		Ino:     inode,
		NameLen: uint32(len(name)),
		Typ:     ModeToType(mode),
	}

	_, err := l.buf.Write(asSlice(unsafe.Pointer(&dirent), unsafe.Sizeof(raw.Dirent{})))
	if err != nil {
		panic("Serialization of Dirent failed")
	}
	l.buf.Write(name)

	padding := 8 - len(name)&7
	if padding < 8 {
		l.buf.Write(make([]byte, padding))
	}

	if l.buf.Len() > l.maxSize {
		l.buf.Truncate(lastLen)
		l.offset--
		return false
	}
	return true
}

func (l *DirEntryList) Bytes() []byte {
	return l.buf.Bytes()
}

////////////////////////////////////////////////////////////////

type rawDir interface {
	ReadDir(input *ReadIn) (*DirEntryList, Status)
	Release()
}

type connectorDir struct {
	node       FsNode 
	stream     []DirEntry
	lastOffset uint64
}

func (d *connectorDir) ReadDir(input *ReadIn) (list *DirEntryList, code Status) {
	if d.stream == nil {
		return nil, OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir(nil)
		if !code.Ok() {
			return nil, code
		}
	}

	off := input.Offset
	list = NewDirEntryList(int(input.Size), off)

	todo := d.stream[off:]
	for _, e := range todo {
		if !list.AddDirEntry(e) {
			break
		}
	}
	d.lastOffset = list.offset
	return list, OK
}

// Read everything so we make goroutines exit.
func (d *connectorDir) Release() {
}
