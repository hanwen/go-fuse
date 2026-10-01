// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Request extensions; see struct fuse_ext_header in linux/fuse.h.
const (
	_MAX_NR_SECCTX = 31
	_EXT_GROUPS    = 32

	sizeOfExtHeader = 8
	sizeOfSecCtx    = 8
)

type secCtx struct {
	name  []byte
	value []byte
}

type requestExt struct {
	suppGroups []uint32
	secctx     []secCtx
}

func (e *requestExt) String() string {
	var parts []string
	if len(e.suppGroups) > 0 {
		parts = append(parts, fmt.Sprintf("groups=%v", e.suppGroups))
	}
	if len(e.secctx) > 0 {
		var ctxs []string
		for _, c := range e.secctx {
			ctxs = append(ctxs, fmt.Sprintf("%s:%q", c.name, bytes.TrimRight(c.value, "\x00")))
		}
		parts = append(parts, "secctx=["+strings.Join(ctxs, " ")+"]")
	}
	return "ext{" + strings.Join(parts, " ") + "}"
}

func align8(n int) int {
	return (n + 7) &^ 7
}

// parse parses b into e, reusing e's slices.
func (e *requestExt) parse(b []byte) Status {
	clear(e.secctx)
	e.secctx = e.secctx[:0]
	e.suppGroups = e.suppGroups[:0]
	for len(b) > 0 {
		if len(b) < sizeOfExtHeader {
			return EIO
		}
		size := binary.NativeEndian.Uint32(b)
		typ := binary.NativeEndian.Uint32(b[4:])
		if size < sizeOfExtHeader || size%8 != 0 || uint64(size) > uint64(len(b)) {
			return EIO
		}
		rec := b[sizeOfExtHeader:size]
		b = b[size:]

		switch {
		case typ <= _MAX_NR_SECCTX:
			if !e.parseSecCtx(rec, int(typ)) {
				return EIO
			}
		case typ == _EXT_GROUPS:
			if len(rec) < 4 {
				return EIO
			}
			nr := binary.NativeEndian.Uint32(rec)
			rec = rec[4:]
			if uint64(nr) > uint64(len(rec)/4) {
				return EIO
			}
			for i := 0; i < int(nr); i++ {
				e.suppGroups = append(e.suppGroups, binary.NativeEndian.Uint32(rec[4*i:]))
			}
		}
	}
	return OK
}

// Each context is fuse_secctx, name\0, value, padded to 8 bytes.
func (e *requestExt) parseSecCtx(rec []byte, nr int) bool {
	for i := 0; i < nr; i++ {
		if len(rec) < sizeOfSecCtx {
			return false
		}
		valSize := binary.NativeEndian.Uint32(rec)
		rest := rec[sizeOfSecCtx:]
		nul := bytes.IndexByte(rest, 0)
		if nul < 0 {
			return false
		}
		name := rest[:nul]
		rest = rest[nul+1:]
		if uint64(valSize) > uint64(len(rest)) {
			return false
		}
		e.secctx = append(e.secctx, secCtx{name: name, value: rest[:valSize]})

		used := sizeOfSecCtx + nul + 1 + int(valSize)
		rec = rec[min(align8(used), len(rec)):]
	}
	return true
}
