// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package winfsp

import (
	"strings"
	"unicode/utf16"
	"unsafe"
)

// Symlinks are IO_REPARSE_TAG_SYMLINK reparse points. As in WinFsp's
// FUSE layer, only relative targets are supported: an absolute POSIX
// target has no meaning on Windows, and an absolute Windows target
// cannot be stored as a POSIX one.

// symlinkFlagRelative is SYMLINK_FLAG_RELATIVE.
const symlinkFlagRelative = 1

// maxReparseDataSize is MAXIMUM_REPARSE_DATA_BUFFER_SIZE.
const maxReparseDataSize = 16 * 1024

// symlinkReparseHeader mirrors REPARSE_DATA_BUFFER for a symlink, up to
// SymbolicLinkReparseBuffer.PathBuffer.
type symlinkReparseHeader struct {
	ReparseTag           uint32
	ReparseDataLength    uint16 // bytes after Reserved
	Reserved             uint16
	SubstituteNameOffset uint16 // relative to PathBuffer
	SubstituteNameLength uint16
	PrintNameOffset      uint16
	PrintNameLength      uint16
	Flags                uint32
}

const symlinkReparseHeaderSize = int(unsafe.Sizeof(symlinkReparseHeader{}))

var _ [20]byte = [symlinkReparseHeaderSize]byte{}

// reparseDataOffset is the offset of the bytes that ReparseDataLength
// counts.
const reparseDataOffset = int(unsafe.Offsetof(symlinkReparseHeader{}.SubstituteNameOffset))

// symlinkReparseData builds a REPARSE_DATA_BUFFER for a symlink with a
// relative target. The path buffer holds the substitute name and the
// print name, which are the same.
func symlinkReparseData(target string) ([]byte, ntStatus) {
	if strings.HasPrefix(target, "/") {
		return nil, _STATUS_ACCESS_DENIED
	}
	u := utf16.Encode([]rune(strings.ReplaceAll(target, "/", `\`)))
	nameBytes := 2 * len(u)
	size := symlinkReparseHeaderSize + 2*nameBytes
	if size > maxReparseDataSize {
		return nil, _STATUS_IO_REPARSE_DATA_INVALID
	}
	b := alignedBuf(size)
	h := (*symlinkReparseHeader)(unsafe.Pointer(&b[0]))
	*h = symlinkReparseHeader{
		ReparseTag:           _IO_REPARSE_TAG_SYMLINK,
		ReparseDataLength:    uint16(size - reparseDataOffset),
		SubstituteNameLength: uint16(nameBytes),
		PrintNameOffset:      uint16(nameBytes),
		PrintNameLength:      uint16(nameBytes),
		Flags:                symlinkFlagRelative,
	}
	copy(utf16At(b, symlinkReparseHeaderSize, len(u)), u)
	copy(utf16At(b, symlinkReparseHeaderSize+nameBytes, len(u)), u)
	return b, _STATUS_SUCCESS
}

// parseSymlinkReparseData returns the target of a symlink
// REPARSE_DATA_BUFFER, with slashes.
func parseSymlinkReparseData(b []byte) (string, ntStatus) {
	if len(b) < 8 {
		return "", _STATUS_IO_REPARSE_DATA_INVALID
	}
	var h symlinkReparseHeader
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&h)), symlinkReparseHeaderSize), b)
	if h.ReparseTag != _IO_REPARSE_TAG_SYMLINK {
		return "", _STATUS_IO_REPARSE_TAG_MISMATCH
	}
	if len(b) < symlinkReparseHeaderSize || reparseDataOffset+int(h.ReparseDataLength) > len(b) {
		return "", _STATUS_IO_REPARSE_DATA_INVALID
	}
	if h.Flags&symlinkFlagRelative == 0 {
		return "", _STATUS_ACCESS_DENIED
	}
	start := symlinkReparseHeaderSize + int(h.SubstituteNameOffset)
	n := int(h.SubstituteNameLength)
	if n == 0 || n%2 != 0 || start+n > reparseDataOffset+int(h.ReparseDataLength) {
		return "", _STATUS_IO_REPARSE_DATA_INVALID
	}
	u := make([]uint16, n/2)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), n), b[start:start+n])
	return strings.ReplaceAll(string(utf16.Decode(u)), `\`, "/"), _STATUS_SUCCESS
}
