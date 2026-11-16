// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/hanwen/go-fuse/v3/fuse"
)

// Windows constants used by WinFsp.
const (
	_FILE_ATTRIBUTE_READONLY      = 0x1
	_FILE_ATTRIBUTE_HIDDEN        = 0x2
	_FILE_ATTRIBUTE_DIRECTORY     = 0x10
	_FILE_ATTRIBUTE_ARCHIVE       = 0x20
	_FILE_ATTRIBUTE_NORMAL        = 0x80
	_FILE_ATTRIBUTE_REPARSE_POINT = 0x400
	_INVALID_FILE_ATTRIBUTES      = 0xFFFFFFFF

	_IO_REPARSE_TAG_SYMLINK = 0xA000000C

	_FILE_DIRECTORY_FILE     = 0x1
	_FILE_NON_DIRECTORY_FILE = 0x40
	_FILE_DELETE_ON_CLOSE    = 0x1000

	_FILE_READ_DATA   = 0x1
	_FILE_WRITE_DATA  = 0x2
	_FILE_APPEND_DATA = 0x4
	_FILE_EXECUTE     = 0x20

	cleanupDelete = 0x1
)

// Linux mode bits, as used on the FUSE wire.
const (
	sIFMT  = 0170000
	sIFDIR = 0040000
	sIFREG = 0100000
	sIFLNK = 0120000
)

// fileInfoSize is sizeof(FSP_FSCTL_FILE_INFO).
const fileInfoSize = 72

// fileInfo mirrors FSP_FSCTL_FILE_INFO.
type fileInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
	AllocationSize uint64
	FileSize       uint64
	CreationTime   uint64
	LastAccessTime uint64
	LastWriteTime  uint64
	ChangeTime     uint64
	IndexNumber    uint64
	HardLinks      uint32
	EaSize         uint32
}

// fileInfo must have the size of FSP_FSCTL_FILE_INFO.
var _ [fileInfoSize]byte = [unsafe.Sizeof(fileInfo{})]byte{}

// volumeInfo holds the fields of FSP_FSCTL_VOLUME_INFO.
type volumeInfo struct {
	TotalSize uint64
	FreeSize  uint64
	Label     string
}

// Seconds between 1601-01-01 and 1970-01-01.
const filetimeEpoch = 11644473600

// filetime converts Unix time to a Windows FILETIME (100ns units since
// 1601).
func filetime(sec uint64, nsec uint32) uint64 {
	return (sec+filetimeEpoch)*1e7 + uint64(nsec)/100
}

// unixTime converts a FILETIME to Unix time.
func unixTime(ft uint64) time.Time {
	t := int64(ft) - filetimeEpoch*1e7
	return time.Unix(t/1e7, (t%1e7)*100)
}

const allocationUnit = 4096

func fileInfoFromAttr(a *fuse.Attr) fileInfo {
	fi := fileInfo{
		FileSize:       a.Size,
		AllocationSize: (a.Size + allocationUnit - 1) / allocationUnit * allocationUnit,
		LastAccessTime: filetime(a.Atime, a.Atimensec),
		LastWriteTime:  filetime(a.Mtime, a.Mtimensec),
		ChangeTime:     filetime(a.Ctime, a.Ctimensec),
		IndexNumber:    a.Ino,
	}
	// FUSE has no birth time outside of STATX.
	fi.CreationTime = fi.LastWriteTime
	switch a.Mode & sIFMT {
	case sIFDIR:
		fi.FileAttributes = _FILE_ATTRIBUTE_DIRECTORY
		fi.FileSize = 0
		fi.AllocationSize = 0
	case sIFLNK:
		fi.FileAttributes = _FILE_ATTRIBUTE_REPARSE_POINT
		fi.ReparseTag = _IO_REPARSE_TAG_SYMLINK
	default:
		fi.FileAttributes = _FILE_ATTRIBUTE_ARCHIVE
	}
	if a.Mode&0222 == 0 && a.Mode&sIFMT != sIFDIR {
		fi.FileAttributes |= _FILE_ATTRIBUTE_READONLY
	}
	return fi
}

// dirInfo mirrors FSP_FSCTL_DIR_INFO, without the file name that
// follows it.
type dirInfo struct {
	Size     uint16
	_        [3]uint16
	FileInfo fileInfo
	_        [24]byte // DirInfoPadding
}

// dirInfoSize is sizeof(FSP_FSCTL_DIR_INFO), without the name.
const dirInfoSize = int(unsafe.Sizeof(dirInfo{}))

var _ [104]byte = [dirInfoSize]byte{}

// maxComponentLength is the longest file name, in UTF-16 code units.
const maxComponentLength = 255

// align8 rounds n up to a multiple of 8, the alignment of entries in
// a directory listing.
func align8(n int) int {
	return (n + 7) &^ 7
}

// alignedBuf returns n zeroed bytes, aligned for the structs that are
// placed in them.
func alignedBuf(n int) []byte {
	w := make([]uint64, (n+7)/8)
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(w))), n)
}

// utf16At returns the n UTF-16 code units at b[off:].
func utf16At(b []byte, off int, n int) []uint16 {
	if n == 0 {
		return nil
	}
	_ = b[off+2*n-1]
	return unsafe.Slice((*uint16)(unsafe.Pointer(&b[off])), n)
}

// appendDirInfo serializes an FSP_FSCTL_DIR_INFO into buf at off. It
// returns the new offset, and false if it did not fit.
func appendDirInfo(buf []byte, off int, name string, fi *fileInfo) (int, bool) {
	u := utf16.Encode([]rune(name))
	size := dirInfoSize + 2*len(u)
	aligned := align8(size)
	if off+aligned > len(buf) {
		return off, false
	}
	b := buf[off : off+aligned]
	clear(b)
	di := (*dirInfo)(unsafe.Pointer(&b[0]))
	di.Size = uint16(size)
	di.FileInfo = *fi
	copy(utf16At(b, dirInfoSize, len(u)), u)
	return off + aligned, true
}

// parseDirInfos parses a ReadDirectory buffer, for tests.
func parseDirInfos(buf []byte) (names []string, infos []fileInfo, end bool) {
	for len(buf) >= 2 {
		// The list ends with a zero Size, without the rest of
		// the struct.
		size := int(*(*uint16)(unsafe.Pointer(&buf[0])))
		if size == 0 {
			return names, infos, true
		}
		di := (*dirInfo)(unsafe.Pointer(&buf[0]))
		u := utf16At(buf, dirInfoSize, (size-dirInfoSize)/2)
		names = append(names, string(utf16.Decode(u)))
		infos = append(infos, di.FileInfo)
		buf = buf[min(len(buf), align8(size)):]
	}
	return names, infos, false
}
