// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"github.com/hanwen/go-fuse/fuse"
)

// InodeEmbed embeds the Inode into a filesystem node. It is the only
// type that implements the InodeLink interface, and hence, it must be
// part of any implementation of Operations.
type InodeEmbed struct {
	inode_ Inode
}

var _ = (InodeLink)((*InodeEmbed)(nil))

func (n *InodeEmbed) inode() *Inode {
	return &n.inode_
}

func (n *InodeEmbed) init(ops InodeLink, attr NodeAttr, bridge *rawBridge, persistent bool) {
	n.inode_ = Inode{
		ops:        ops,
		nodeAttr:   attr,
		bridge:     bridge,
		persistent: persistent,
		parents:    make(map[parentData]struct{}),
	}
	if attr.Mode == fuse.S_IFDIR {
		n.inode_.children = make(map[string]*Inode)
	}
}

// Inode returns the Inode for this Operations
func (n *InodeEmbed) Inode() *Inode {
	return &n.inode_
}
