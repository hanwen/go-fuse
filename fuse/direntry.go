package fuse

// all of the code for DirEntryList.

import (
	"encoding/binary"
	"fmt"
	"bytes"
)

// For PathFileSystemConnector.  The connector determines inodes.
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

func (de *DirEntryList) AddString(name string, inode uint64, mode uint32) bool {
	return de.Add([]byte(name), inode, mode)
}

func (de *DirEntryList) Add(name []byte, inode uint64, mode uint32) bool {
	lastLen := de.buf.Len()
	de.offset++

	dirent := new(Dirent)
	dirent.Off = de.offset
	dirent.Ino = inode
	dirent.NameLen = uint32(len(name))
	dirent.Typ = ModeToType(mode)

	err := binary.Write(&de.buf, binary.LittleEndian, dirent)
	if err != nil {
		panic("Serialization of Dirent failed")
	}
	de.buf.Write(name)

	padding := 8 - len(name)&7
	if padding < 8 {
		de.buf.Write(make([]byte, padding))
	}

	if de.buf.Len() > de.maxSize {
		de.buf.Truncate(lastLen)
		de.offset--
		return false
	}
	return true
}

func (de *DirEntryList) Bytes() []byte {
	return de.buf.Bytes()
}

////////////////////////////////////////////////////////////////

type FuseDir struct {
	stream chan DirEntry
	leftOver DirEntry
	connector *PathFileSystemConnector
	parentIno uint64
}

func (me *FuseDir) inode(name string) uint64 {
	// We could also return
	// me.connector.lookupUpdate(me.parentIno, name).NodeId but it
	// appears FUSE will issue a LOOKUP afterwards for the entry
	// anyway, so we skip hash table update here.
	return FUSE_UNKNOWN_INO
}

func (me *FuseDir) ReadDir(input *ReadIn) (*DirEntryList, Status) {
	list := NewDirEntryList(int(input.Size))

	if me.leftOver.Name != "" {
		n := me.leftOver.Name
		i := me.inode(n)
		success := list.AddString(n, i, me.leftOver.Mode)
		if !success {
			panic("No space for single entry.")
		}
		me.leftOver.Name = ""
	}

	for {
		d := <-me.stream
		if d.Name == "" {
			break
		}
		i := me.inode(d.Name)

		if !list.AddString(d.Name, i, d.Mode) {
			me.leftOver = d
			break
		}
	}
	return list, OK
}

func (me *FuseDir) ReleaseDir() {
	close(me.stream)
}

func (me *FuseDir) FsyncDir(input *FsyncIn) (code Status) {
	return ENOSYS
}
