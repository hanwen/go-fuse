// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchmark

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type StatFS struct {
	fs.Inode

	files map[string]fuse.Attr
}

var _ = (fs.NodeOnAdder)((*StatFS)(nil))

func (r *StatFS) OnAdd(ctx context.Context) {
	for nm, a := range r.files {
		r.addFile(nm, a)
	}
	r.files = nil
}

func (r *StatFS) AddFile(name string, a fuse.Attr) {
	if r.files == nil {
		r.files = map[string]fuse.Attr{}
	}

	r.files[name] = a
}

func (r *StatFS) addFile(name string, a fuse.Attr) {
	dir, base := filepath.Split(name)

	p := r.EmbeddedInode()

	// Add directories leading up to the file.
	for _, component := range strings.Split(dir, "/") {
		if len(component) == 0 {
			continue
		}
		ch := p.GetChild(component)
		if ch == nil {
			// Create a directory
			ch = p.NewPersistentInode(context.Background(), &fs.Inode{},
				fs.StableAttr{Mode: syscall.S_IFDIR})
			// Add it
			p.AddChild(component, ch, true)
		}

		p = ch
	}

	// Create the file
	child := p.NewPersistentInode(context.Background(), &fs.MemRegularFile{
		Data: make([]byte, a.Size),
		Attr: a,
	}, fs.StableAttr{})

	// And add it
	p.AddChild(base, child, true)
}
