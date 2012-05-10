package fuse

// all of the code for DirEntryList.

import (
	"bytes"
	"fmt"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var _ = fmt.Print

// DirEntry is a type for PathFileSystem and NodeFileSystem to return
// directory contents in.
type DirEntry struct {
	Mode uint32
	Name string
}

type DirEntryList struct {
	buf     bytes.Buffer
	offset  *uint64
	maxSize int
}

func NewDirEntryList(max int, off *uint64) *DirEntryList {
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
	(*l.offset)++

	dirent := raw.Dirent{
		Off:     *l.offset,
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
		(*l.offset)--
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
	stream     []DirEntry
	leftOver   DirEntry
	lastOffset uint64
}

// TODO - use index into []stream for seeking correctly.
func (d *connectorDir) ReadDir(input *ReadIn) (*DirEntryList, Status) {
	if d.stream == nil {
		return nil, OK
	}

	list := NewDirEntryList(int(input.Size), &d.lastOffset)
	if d.leftOver.Name != "" {
		success := list.AddDirEntry(d.leftOver)
		if !success {
			panic("No space for single entry.")
		}
		d.leftOver.Name = ""
	}
	for len(d.stream) > 0 {
		e := d.stream[len(d.stream)-1]
		success := list.AddDirEntry(e)
		if !success {
			return list, OK
		}
		d.stream = d.stream[:len(d.stream)-1]
	}
	return list, OK
}

// Read everything so we make goroutines exit.
func (d *connectorDir) Release() {
}
