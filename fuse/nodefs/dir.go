package nodefs

import (
	"log"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/raw"
)

type connectorDir struct {
	node       Node
	stream     []fuse.DirEntry
	lastOffset uint64
	rawFS      fuse.RawFileSystem
	lookups    []raw.EntryOut
}

func (d *connectorDir) ReadDir(list *fuse.DirEntryList, input *raw.ReadIn, context *fuse.Context) (code fuse.Status) {
	if d.stream == nil {
		return fuse.OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir(context)
		if !code.Ok() {
			return code
		}
	}

	todo := d.stream[input.Offset:]
	for _, e := range todo {
		if e.Name == "" {
			log.Printf("got emtpy directory entry, mode %o.", e.Mode)
			continue
		}
		if !list.AddDirEntry(e) {
			break
		}
	}
	d.lastOffset = list.Offset
	return fuse.OK
}

func (d *connectorDir) ReadDirPlus(list *fuse.DirEntryList, input *raw.ReadIn, context *fuse.Context) (code fuse.Status) {
	if d.stream == nil {
		return fuse.OK
	}

	// rewinddir() should be as if reopening directory.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir(context)
		if !code.Ok() {
			return code
		}
		d.lookups = nil
	}

	if d.lookups == nil {
		d.lookups = make([]raw.EntryOut, len(d.stream))
		for i, n := range d.stream {
			if n.Name == "." || n.Name == ".." {
				continue
			}
			// We ignore the return value
			code := d.rawFS.Lookup(&d.lookups[i], context, n.Name)
			if !code.Ok() {
				d.lookups[i] = raw.EntryOut{}
			}
		}
	}

	todo := d.stream[input.Offset:]
	for i, e := range todo {
		if e.Name == "" {
			log.Printf("got empty directory entry, mode %o.", e.Mode)
			continue
		}
		if !list.AddDirLookupEntry(e, &d.lookups[input.Offset+uint64(i)]) {
			break
		}
	}
	d.lastOffset = list.Offset
	return fuse.OK

}

type rawDir interface {
	ReadDir(out *fuse.DirEntryList, input *raw.ReadIn, c *fuse.Context) fuse.Status
	ReadDirPlus(out *fuse.DirEntryList, input *raw.ReadIn, c *fuse.Context) fuse.Status
}
