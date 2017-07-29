// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sync"

	"github.com/hanwen/go-fuse/fuse"
)

type connectorDir struct {
	node  Node
	rawFS fuse.RawFileSystem

	// Protect stream and lastOffset.  These are written in case
	// there is a seek on the directory.
	mu     sync.Mutex
	stream []fuse.DirEntryIno

	// lastOffset stores the last offset for a readdir. This lets
	// readdir pick up changes to the directory made after opening
	// it.
	lastOffset uint64
}

// openDirIno calls OpenDir or, if OpenDir returns ENOSYS, OpenDirIno.
// If OpenDir returns success, the inode number is set to FUSE_UNKNOWN_INO
// on all entries.
func openDirIno(node Node, context *fuse.Context) ([]fuse.DirEntryIno, fuse.Status) {
	entries, code := node.OpenDir(context)
	if code == fuse.ENOSYS {
		return node.OpenDirIno(context)
	}
	if !code.Ok() {
		return nil, code
	}
	entriesIno := make([]fuse.DirEntryIno, len(entries))
	for i, v := range entries {
		entriesIno[i] = fuse.DirEntryIno{v.Mode, v.Name, fuse.FUSE_UNKNOWN_INO}
	}
	return entriesIno, fuse.OK
}

func (d *connectorDir) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stream == nil {
		return fuse.OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = openDirIno(d.node, (*fuse.Context)(&input.Context))
		if !code.Ok() {
			return code
		}
	}

	if input.Offset > uint64(len(d.stream)) {
		// This shouldn't happen, but let's not crash.
		return fuse.EINVAL
	}

	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}
		ok, off := out.AddDirEntryIno(e)
		d.lastOffset = off
		if !ok {
			break
		}
	}
	return fuse.OK
}

func (d *connectorDir) ReadDirPlus(input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stream == nil {
		return fuse.OK
	}

	// rewinddir() should be as if reopening directory.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = openDirIno(d.node, (*fuse.Context)(&input.Context))
		if !code.Ok() {
			return code
		}
	}

	if input.Offset > uint64(len(d.stream)) {
		// This shouldn't happen, but let's not crash.
		return fuse.EINVAL
	}
	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}

		// we have to be sure entry will fit if we try to add
		// it, or we'll mess up the lookup counts.
		entryDest, off := out.AddDirLookupEntryIno(e)
		if entryDest == nil {
			break
		}
		entryDest.Ino = uint64(fuse.FUSE_UNKNOWN_INO)

		// No need to fill attributes for . and ..
		if e.Name == "." || e.Name == ".." {
			continue
		}

		// Clear entryDest before use it, some fields can be corrupted if does not set all fields in rawFS.Lookup
		*entryDest = fuse.EntryOut{}

		d.rawFS.Lookup(&input.InHeader, e.Name, entryDest)
		d.lastOffset = off
	}
	return fuse.OK

}

type rawDir interface {
	ReadDir(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
	ReadDirPlus(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
}
