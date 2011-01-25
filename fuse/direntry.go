package fuse

// all of the code for DirEntryList.

import (
	"encoding/binary"
	"bytes"
)

// Should make interface ?
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
