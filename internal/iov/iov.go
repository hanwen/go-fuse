// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iov

import "bytes"

// Len returns the total byte length across all slices in v.
func Len(v [][]byte) int {
	var r int
	for _, e := range v {
		r += len(e)
	}
	return r
}

// Lens returns the byte length of each slice in v.
func Lens(v [][]byte) []int {
	var lens []int
	for _, b := range v {
		lens = append(lens, len(b))
	}
	return lens
}

// Slice returns v[off:off+n] if it lies within a single element.
func Slice(v [][]byte, off, n int) ([]byte, bool) {
	for _, e := range v {
		if off < len(e) {
			if off+n <= len(e) {
				return e[off : off+n], true
			}
			return nil, false
		}
		off -= len(e)
	}
	return nil, n == 0
}

// Subslice returns the sub-iov for bytes [off, off+n).
func Subslice(v [][]byte, off, n int) [][]byte {
	var r [][]byte
	for _, e := range v {
		if n == 0 {
			break
		}
		if off >= len(e) {
			off -= len(e)
			continue
		}
		e = e[off:]
		off = 0
		e = e[:min(len(e), n)]
		n -= len(e)
		r = append(r, e)
	}
	return r
}

// CopyFrom copies bytes from v starting at off into dst.
func CopyFrom(dst []byte, v [][]byte, off int) int {
	var n int
	for _, e := range v {
		if len(dst) == n {
			break
		}
		if off >= len(e) {
			off -= len(e)
			continue
		}
		n += copy(dst[n:], e[off:])
		off = 0
	}
	return n
}

// CopyTo copies bytes from src slices sequentially into dst.
func CopyTo(dst [][]byte, src ...[]byte) int {
	w := Writer{Dst: dst}
	for _, s := range src {
		w.Write(s)
	}
	return w.Written
}

// HasPrefixAt reports whether the byte stream in v starting at off begins with prefix.
func HasPrefixAt(v [][]byte, off int, prefix []byte) bool {
	for _, b := range v {
		if off >= len(b) {
			off -= len(b)
			continue
		}
		avail := b[off:]
		if len(avail) >= len(prefix) {
			return bytes.HasPrefix(avail, prefix)
		}
		if !bytes.HasPrefix(prefix, avail) {
			return false
		}
		prefix = prefix[len(avail):]
		off = 0
	}
	return len(prefix) == 0
}

// Writer writes sequentially into a scatter-gather slice list.
type Writer struct {
	Dst     [][]byte
	i, off  int
	Written int
}

func (w *Writer) next(n int) []byte {
	for w.i < len(w.Dst) && w.off == len(w.Dst[w.i]) {
		w.i++
		w.off = 0
	}
	if w.i == len(w.Dst) {
		return nil
	}
	chunk := w.Dst[w.i][w.off:]
	chunk = chunk[:min(len(chunk), n)]
	w.off += len(chunk)
	w.Written += len(chunk)
	return chunk
}

// Write copies src into the remaining capacity of w.Dst.
func (w *Writer) Write(src []byte) int {
	var n int
	for n < len(src) {
		chunk := w.next(len(src) - n)
		if chunk == nil {
			break
		}
		n += copy(chunk, src[n:])
	}
	return n
}

// Skip advances w by n bytes without writing.
func (w *Writer) Skip(n int) {
	for n > 0 {
		chunk := w.next(n)
		if chunk == nil {
			break
		}
		n -= len(chunk)
	}
}
