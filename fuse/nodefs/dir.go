// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sync"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type connectorDir struct {
	node  Node
	inode *Inode
	rawFS fuse.RawFileSystem

	// Protect stream and lastOffset.  These are written in case
	// there is a seek on the directory.
	mu     sync.Mutex
	stream []fuse.DirEntry
}

func (d *connectorDir) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.stream == nil || input.Offset == 0 {
		d.stream, code = d.node.OpenDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
		if !code.Ok() {
			return code
		}
		d.stream = append(d.stream, d.inode.getMountDirEntries()...)
		d.stream = append(d.stream,
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: "."},
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: ".."})
	}

	if input.Offset > uint64(len(d.stream)) {
		// See https://github.com/hanwen/go-fuse/issues/297
		// This can happen for FUSE exported over NFS.  This
		// seems incorrect, (maybe the kernel is using offsets
		// from other opendir/readdir calls), it is harmless to reinforce that
		// we have reached EOF.
		return fuse.OK
	}

	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}
		ok := out.AddDirEntry(e)
		if !ok {
			break
		}
	}
	return fuse.OK
}

func (d *connectorDir) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// rewinddir() should be as if reopening directory.
	if d.stream == nil || input.Offset == 0 {
		d.stream, code = d.node.OpenDir(&fuse.Context{Caller: input.Caller, Cancel: cancel})
		if !code.Ok() {
			return code
		}
		d.stream = append(d.stream, d.inode.getMountDirEntries()...)
		d.stream = append(d.stream,
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: "."},
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: ".."})
	}

	if input.Offset > uint64(len(d.stream)) {
		// See comment in Readdir
		return fuse.OK
	}
	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}

		// we have to be sure entry will fit if we try to add
		// it, or we'll mess up the lookup counts.
		entryDest := out.AddDirLookupEntry(e)
		if entryDest == nil {
			break
		}
		entryDest.Ino = uint64(fuse.FUSE_UNKNOWN_INO)

		// No need to fill attributes for . and ..
		if e.Name == "." || e.Name == ".." {
			continue
		}

		d.rawFS.Lookup(cancel, &input.InHeader, e.Name, entryDest)
	}
	return fuse.OK
}

type rawDir interface {
	ReadDir(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
	ReadDirPlus(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
}
