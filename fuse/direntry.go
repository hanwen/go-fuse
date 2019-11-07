// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

// all of the code for DirEntryList.

import (
	"fmt"
	"unsafe"
)

var eightPadding [8]byte

const direntSize = int(unsafe.Sizeof(_Dirent{}))

// DirEntry is a type for PathFileSystem and NodeFileSystem to return
// directory contents in.
type DirEntry struct {
	// Mode is the file's mode. Only the high bits (eg. S_IFDIR)
	// are considered.
	Mode uint32

	// Name is the basename of the file in the directory.
	Name string

	// Ino is the inode number.
	Ino uint64
}

func (d DirEntry) String() string {
	return fmt.Sprintf("%o: %q ino=%d", d.Mode, d.Name, d.Ino)
}

// DirEntryList holds the return value for READDIR and READDIRPLUS
// opcodes.
type DirEntryList struct {
	buf    []byte
	size   int    // capacity of the underlying buffer
	offset uint64 // entry count (NOT a byte offset)
}

// NewDirEntryList creates a DirEntryList with the given data buffer
// and offset.
func NewDirEntryList(data []byte, off uint64) *DirEntryList {
	return &DirEntryList{
		buf:    data[:0],
		size:   len(data),
		offset: off,
	}
}

// AddDirEntry tries to add an entry, and reports whether it
// succeeded.
func (l *DirEntryList) AddDirEntry(e DirEntry) bool {
	return l.Add(0, e.Name, e.Ino, e.Mode)
}

// Add adds a direntry to the DirEntryList, returning whether it
// succeeded.
func (l *DirEntryList) Add(prefix int, name string, inode uint64, mode uint32) bool {
	if inode == 0 {
		inode = FUSE_UNKNOWN_INO
	}
	padding := (8 - len(name)&7) & 7
	delta := padding + direntSize + len(name) + prefix
	oldLen := len(l.buf)
	newLen := delta + oldLen

	if newLen > l.size {
		return false
	}
	l.buf = l.buf[:newLen]
	oldLen += prefix
	dirent := (*_Dirent)(unsafe.Pointer(&l.buf[oldLen]))
	dirent.Off = l.offset + 1
	dirent.Ino = inode
	dirent.NameLen = uint32(len(name))
	dirent.Typ = (mode & 0170000) >> 12
	oldLen += direntSize
	copy(l.buf[oldLen:], name)
	oldLen += len(name)

	if padding > 0 {
		copy(l.buf[oldLen:], eightPadding[:padding])
	}

	l.offset = dirent.Off
	return true
}

// AddDirLookupEntry is used for ReadDirPlus. If reserves and zeroizes space
// for an EntryOut struct and serializes a DirEntry.
// On success, it returns pointers to both structs.
// If not enough space was left, it returns two nil pointers.
//
// The resulting READDIRPLUS output buffer looks like this in memory:
// 1) EntryOut{}
// 2) _Dirent{}
// 3) Name (null-terminated)
// 4) Padding to align to 8 bytes
// [repeat]
func (l *DirEntryList) AddDirLookupEntry(e DirEntry) (*EntryOut, *_Dirent) {
	const entryOutSize = int(unsafe.Sizeof(EntryOut{}))
	oldLen := len(l.buf)
	ok := l.Add(entryOutSize, e.Name, e.Ino, e.Mode)
	if !ok {
		return nil, nil
	}
	entryOut := (*EntryOut)(unsafe.Pointer(&l.buf[oldLen]))
	*entryOut = EntryOut{} // zeroize
	dirent := (*_Dirent)(unsafe.Pointer(&l.buf[oldLen+entryOutSize]))
	return entryOut, dirent
}

func (l *DirEntryList) bytes() []byte {
	return l.buf
}
