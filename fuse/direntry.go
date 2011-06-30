package fuse

// all of the code for DirEntryList.

import (
	"bytes"
	"fmt"
	"unsafe"
)

var _ = fmt.Print
// For FileSystemConnector.  The connector determines inodes.
type DirEntry struct {
	Mode uint32
	Name string
}

type DirEntryList struct {
	buf     bytes.Buffer
	offset  uint64
	maxSize int
}

func NewDirEntryList(max int) *DirEntryList {
	return &DirEntryList{maxSize: max}
}

func (me *DirEntryList) AddString(name string, inode uint64, mode uint32) bool {
	return me.Add([]byte(name), inode, mode)
}

func (me *DirEntryList) Add(name []byte, inode uint64, mode uint32) bool {
	lastLen := me.buf.Len()
	me.offset++

	dirent := Dirent{
		Off:     me.offset,
		Ino:     inode,
		NameLen: uint32(len(name)),
		Typ:     ModeToType(mode),
	}

	_, err := me.buf.Write(asSlice(unsafe.Pointer(&dirent), unsafe.Sizeof(Dirent{})))
	if err != nil {
		panic("Serialization of Dirent failed")
	}
	me.buf.Write(name)

	padding := 8 - len(name)&7
	if padding < 8 {
		me.buf.Write(make([]byte, padding))
	}

	if me.buf.Len() > me.maxSize {
		me.buf.Truncate(lastLen)
		me.offset--
		return false
	}
	return true
}

func (me *DirEntryList) Bytes() []byte {
	return me.buf.Bytes()
}

////////////////////////////////////////////////////////////////

type Dir struct {
	extra    []DirEntry
	stream   chan DirEntry
	leftOver DirEntry
}

func (me *Dir) ReadDir(input *ReadIn) (*DirEntryList, Status) {
	if me.stream == nil && len(me.extra) == 0 {
		return nil, OK
	}

	// We could also return
	// me.connector.lookupUpdate(me.parentIno, name).NodeId but it
	// appears FUSE will issue a LOOKUP afterwards for the entry
	// anyway, so we skip hash table update here.
	inode := uint64(FUSE_UNKNOWN_INO)

	list := NewDirEntryList(int(input.Size))
	if me.leftOver.Name != "" {
		n := me.leftOver.Name
		success := list.AddString(n, inode, me.leftOver.Mode)
		if !success {
			panic("No space for single entry.")
		}
		me.leftOver.Name = ""
	}
	for len(me.extra) > 0 {
		e := me.extra[len(me.extra)-1]
		me.extra = me.extra[:len(me.extra)-1]
		success := list.AddString(e.Name, inode, e.Mode)
		if !success {
			me.leftOver = e
			return list, OK
		}
	}
	for {
		d, isOpen := <-me.stream
		if !isOpen {
			me.stream = nil
			break
		}
		if !list.AddString(d.Name, inode, d.Mode) {
			me.leftOver = d
			break
		}
	}
	return list, OK
}

// Read everything so we make goroutines exit.
func (me *Dir) Release() {
	for ok := true; ok && me.stream != nil; {
		_, ok = <-me.stream
		if !ok {
			break
		}
	}
}
