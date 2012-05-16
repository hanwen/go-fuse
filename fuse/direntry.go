package fuse

// all of the code for DirEntryList.

import (
	"bytes"
	"log"
	"unsafe"

	"github.com/hanwen/go-fuse/raw"
)

var _ = log.Print
var eightPadding [8]byte

// DirEntry is a type for PathFileSystem and NodeFileSystem to return
// directory contents in.
type DirEntry struct {
	Mode uint32
	Name string
}

type DirEntryList struct {
	buf     *bytes.Buffer
	offset  uint64
	maxSize int
}

func NewDirEntryList(data []byte, off uint64) *DirEntryList {
	return &DirEntryList{
		buf: bytes.NewBuffer(data[:0]),
		maxSize: len(data),
		offset: off,
	}
}

func (l *DirEntryList) AddString(name string, inode uint64, mode uint32) bool {
	return l.Add([]byte(name), inode, mode)
}

func (l *DirEntryList) AddDirEntry(e DirEntry) bool {
	return l.Add([]byte(e.Name), uint64(raw.FUSE_UNKNOWN_INO), e.Mode)
}

func (l *DirEntryList) Add(name []byte, inode uint64, mode uint32) bool {
	dirent := raw.Dirent{
		Off:     l.offset+1,
		Ino:     inode,
		NameLen: uint32(len(name)),
		Typ:     ModeToType(mode),
	}

	padding := 8 - len(name)&7
	if padding == 8 {
		padding = 0
	}
	
	delta := padding + int(unsafe.Sizeof(raw.Dirent{})) + len(name)
	newLen := delta + l.buf.Len()
	if newLen > l.maxSize {
		return false
	}
	_, err := l.buf.Write(asSlice(unsafe.Pointer(&dirent), unsafe.Sizeof(raw.Dirent{})))
	if err != nil {
		panic("Serialization of Dirent failed")
	}
	l.buf.Write(name)
	if padding > 0 {
		l.buf.Write(eightPadding[:padding])
	}
	l.offset = dirent.Off

	if l.buf.Len() != newLen {
		log.Panicf("newLen mismatch %d %d", l.buf.Len(), newLen)
	}
	return true
}

func (l *DirEntryList) Bytes() []byte {
	return l.buf.Bytes()
}

////////////////////////////////////////////////////////////////

type rawDir interface {
	ReadDir(out *DirEntryList, input *ReadIn) (Status)
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
