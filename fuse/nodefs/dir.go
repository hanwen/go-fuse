package nodefs

import (
	"log"

	"github.com/hanwen/go-fuse/fuse"
)

type connectorDir struct {
	node       Node
	stream     []fuse.DirEntry
	lastOffset uint64
	rawFS      fuse.RawFileSystem
	lookups    []fuse.EntryOut
}

func (d *connectorDir) ReadDir(input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	if d.stream == nil {
		return fuse.OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir((*fuse.Context)(&input.Context))
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
			log.Printf("got emtpy directory entry, mode %o.", e.Mode)
			continue
		}
		ok, off := out.AddDirEntry(e)
		d.lastOffset = off
		if !ok {
			break
		}
	}
	return fuse.OK
}

func (d *connectorDir) ReadDirPlus(input *fuse.ReadIn, out *fuse.DirEntryList) (code fuse.Status) {
	if d.stream == nil {
		return fuse.OK
	}

	// rewinddir() should be as if reopening directory.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir((*fuse.Context)(&input.Context))
		if !code.Ok() {
			return code
		}
		d.lookups = nil
	}

	if d.lookups == nil {
		d.lookups = make([]fuse.EntryOut, len(d.stream))
		for i, n := range d.stream {
			if n.Name == "." || n.Name == ".." {
				continue
			}
			// We ignore the return value
			code := d.rawFS.Lookup(&input.InHeader, n.Name, &d.lookups[i])
			if !code.Ok() {
				d.lookups[i] = fuse.EntryOut{}
			}
		}
	}

	if input.Offset > uint64(len(d.stream)) {
		// This shouldn't happen, but let's not crash.
		return fuse.EINVAL
	}
	todo := d.stream[input.Offset:]
	for i, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}
		ok, off := out.AddDirLookupEntry(e, &d.lookups[input.Offset+uint64(i)])
		d.lastOffset = off
		if !ok {
			break
		}
	}
	return fuse.OK

}

type rawDir interface {
	ReadDir(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
	ReadDirPlus(out *fuse.DirEntryList, input *fuse.ReadIn, c *fuse.Context) fuse.Status
}
