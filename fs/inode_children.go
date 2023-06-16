// Copyright 2023 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"fmt"
	"strings"
	"unsafe"
)

type childEntry struct {
	Name  string
	Inode *Inode
}

type inodeChildren struct {
	children map[string]*Inode
}

func (c *inodeChildren) init() {
	c.children = make(map[string]*Inode)
}

func (c *inodeChildren) String() string {
	var ss []string
	for nm, ch := range c.children {
		ss = append(ss, fmt.Sprintf("%q=i%d[%s]", nm, ch.stableAttr.Ino, modeStr(ch.stableAttr.Mode)))
	}
	return strings.Join(ss, ",")
}

func (c *inodeChildren) get(name string) *Inode {
	return c.children[name]
}

func (c *inodeChildren) set(parent *Inode, name string, ch *Inode) {
	c.children[name] = ch
	parent.changeCounter++

	ch.parents.add(parentData{name, parent})
	ch.changeCounter++
}

func (c *inodeChildren) len() int {
	return len(c.children)
}

func (c *inodeChildren) toMap() map[string]*Inode {
	r := make(map[string]*Inode, len(c.children))
	for k, v := range c.children {
		r[k] = v
	}
	return r
}

func (c *inodeChildren) del(parent *Inode, name string) {
	ch := c.children[name]
	if ch == nil {
		return
	}

	delete(c.children, name)
	ch.parents.delete(parentData{name, parent})
	ch.changeCounter++
	parent.changeCounter++
}

func (c *inodeChildren) list() []childEntry {
	r := make([]childEntry, 0, 2*len(c.children))

	// The spec doesn't guarantee this, but as long as maps remain
	// backed by hash tables, the simplest mechanism for
	// randomization is picking a random start index. We undo this
	// here by picking a deterministic start index again. If the
	// Go runtime ever implements a memory moving GC, we might
	// have to look at the keys instead.
	minNode := ^uintptr(0)
	minIdx := -1
	for k, v := range c.children {
		if p := uintptr(unsafe.Pointer(v)); p < minNode {
			minIdx = len(r)
			minNode = p
		}
		r = append(r, childEntry{Name: k, Inode: v})
	}

	if minIdx > 0 {
		r = append(r[minIdx:], r[:minIdx]...)
	}
	return r

}
