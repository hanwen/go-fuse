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
}

func (d *connectorDir) ReadDir(list *fuse.DirEntryList, input *raw.ReadIn) (code fuse.Status) {
	if d.stream == nil {
		return fuse.OK
	}
	// rewinddir() should be as if reopening directory.
	// TODO - test this.
	if d.lastOffset > 0 && input.Offset == 0 {
		d.stream, code = d.node.OpenDir(nil)
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

// Read everything so we make goroutines exit.
func (d *connectorDir) Release() {
}

type rawDir interface {
	ReadDir(out *fuse.DirEntryList, input *raw.ReadIn) fuse.Status
	Release()
}
