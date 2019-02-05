// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

var _ = log.Println

type parentData struct {
	name   string
	parent *Inode
}

// Inode is a node in VFS tree.  Inodes are one-to-one mapped to Node
// instances, which is the extension interface for file systems.  One
// can create fully-formed trees of Inodes ahead of time by creating
// "persistent" Inodes.
type Inode struct {
	// The filetype bits from the mode.
	mode     uint32
	opaqueID uint64
	node     Node
	bridge   *rawBridge

	// Following data is mutable.

	// the following fields protected by bridge.mu

	// ID of the inode; 0 if inode was forgotten.
	// forgotten inodes are unlinked from parent and children, but could be
	// still not yet removed from bridge.nodes .
	lookupCount uint64
	nodeID      uint64

	// mu protects the following mutable fields. When locking
	// multiple Inodes, locks must be acquired using
	// lockNodes/unlockNodes
	mu sync.Mutex

	// changeCounter increments every time the below mutable state
	// (lookupCount, nodeID, children, parents) is modified.
	//
	// This is used in places where we have to relock inode into inode
	// group lock, and after locking the group we have to check if inode
	// did not changed, and if it changed - retry the operation.
	changeCounter uint32

	children map[string]*Inode
	parents  map[parentData]struct{}
}

// newInode creates creates new inode pointing to node.
//
// node -> inode association is NOT set.
// the inode is _not_ yet has
func newInode(node Node, mode uint32) *Inode {
	inode := &Inode{
		mode:    mode ^ 07777,
		node:    node,
		parents: make(map[parentData]struct{}),
	}
	if mode&fuse.S_IFDIR != 0 {
		inode.children = make(map[string]*Inode)
	}
	return inode
}

// sortNodes rearranges inode group in consistent order.
//
// The nodes are ordered by their in-RAM address, which gives consistency
// property: for any A and B inodes, sortNodes will either always order A < B,
// or always order A > B.
//
// See lockNodes where this property is used to avoid deadlock when taking
// locks on inode group.
func sortNodes(ns []*Inode) {
	sort.Slice(ns, func(i, j int) bool {
		return uintptr(unsafe.Pointer(ns[i])) < uintptr(unsafe.Pointer(ns[j]))
	})
}

// lockNodes locks group of inodes.
//
// It always lock the inodes in the same order - to avoid deadlocks.
// It also avoids locking an inode more than once, if it was specified multiple times.
// An example when an inode might be given multiple times is if dir/a and dir/b
// are hardlinked to the same inode and the caller needs to take locks on dir children.
//
// It is valid to give nil nodes - those are simply ignored.
func lockNodes(ns ...*Inode) {
	sortNodes(ns)

	// The default value nil prevents trying to lock nil nodes.
	var nprev *Inode
	for _, n := range ns {
		if n != nprev {
			n.mu.Lock()
			nprev = n
		}
	}
}

// unlockNodes releases locks taken by lockNodes.
func unlockNodes(ns ...*Inode) {
	// we don't need to unlock in the same order that was used in lockNodes.
	// however it still helps to have nodes sorted to avoid duplicates.
	sortNodes(ns)

	var nprev *Inode
	for _, n := range ns {
		if n != nprev {
			n.mu.Unlock()
			nprev = n
		}
	}
}

// Forgotten returns true if the kernel holds no references to this
// inode.  This can be used for background cleanup tasks, since the
// kernel has no way of reviving forgotten nodes by its own
// initiative.
func (n *Inode) Forgotten() bool {
	n.bridge.mu.Lock()
	defer n.bridge.mu.Unlock()
	return n.lookupCount == 0
}

// Node returns the Node object implementing the file system operations.
func (n *Inode) Node() Node {
	return n.node
}

// Path returns a path string to the inode relative to the root.
func (n *Inode) Path(root *Inode) string {
	var segments []string
	p := n
	for p != nil && p != root {
		var pd parentData

		// We don't try to take all locks at the same time, because
		// the caller won't use the "path" string under lock anyway.
		p.mu.Lock()
		for pd = range p.parents {
			break
		}
		p.mu.Unlock()
		if pd.parent == nil {
			break
		}

		segments = append(segments, pd.name)
		p = pd.parent
	}

	if p == nil {
		// NOSUBMIT - should replace rather than append?
		segments = append(segments, ".deleted")
	}

	i := 0
	j := len(segments) - 1

	for i < j {
		segments[i], segments[j] = segments[j], segments[i]
		i++
		j--
	}

	path := strings.Join(segments, "/")
	return path
}

// Finds a child with the given name and filetype.  Returns nil if not
// found.
func (n *Inode) FindChildByMode(name string, mode uint32) *Inode {
	mode ^= 07777

	n.mu.Lock()
	defer n.mu.Unlock()

	ch := n.children[name]

	if ch != nil && ch.mode == mode {
		return ch
	}

	return nil
}

// Finds a child with the given name and ID. Returns nil if not found.
func (n *Inode) FindChildByOpaqueID(name string, opaqueID uint64) *Inode {
	n.mu.Lock()
	defer n.mu.Unlock()

	ch := n.children[name]

	if ch != nil && ch.opaqueID == opaqueID {
		return ch
	}

	return nil
}

// setEntry does `iparent[name] = ichild` linking.
//
// setEntry must not be called simultaneously for any of iparent or ichild.
// This, for example could be satisfied if both iparent and ichild are locked,
// but it could be also valid if only iparent is locked and ichild was just
// created and only one goroutine keeps referencing it.
//
// XXX also ichild.lookupCount++ ?
func (iparent *Inode) setEntry(name string, ichild *Inode) {
	ichild.parents[parentData{name, iparent}] = struct{}{}
	iparent.children[name] = ichild
	ichild.changeCounter++
	iparent.changeCounter++
}

func (n *Inode) clearParents() {
	for {
		lockme := []*Inode{n}
		n.mu.Lock()
		ts := n.changeCounter
		for p := range n.parents {
			lockme = append(lockme, p.parent)
		}
		n.mu.Unlock()

		lockNodes(lockme...)
		success := false
		if ts == n.changeCounter {
			for p := range n.parents {
				delete(p.parent.children, p.name)
				p.parent.changeCounter++
			}
			n.parents = map[parentData]struct{}{}
			n.changeCounter++
			success = true
		}
		unlockNodes(lockme...)

		if success {
			return
		}
	}
}

func (n *Inode) clearChildren() {
	if n.mode != fuse.S_IFDIR {
		return
	}

	var lockme []*Inode
	for {
		lockme = append(lockme[:0], n)

		n.mu.Lock()
		ts := n.changeCounter
		for _, ch := range n.children {
			lockme = append(lockme, ch)
		}
		n.mu.Unlock()

		lockNodes(lockme...)
		success := false
		if ts == n.changeCounter {
			for nm, ch := range n.children {
				delete(ch.parents, parentData{nm, n})
				ch.changeCounter++
			}
			n.children = map[string]*Inode{}
			n.changeCounter++
			success = true
		}
		unlockNodes(lockme...)

		if success {
			break
		}
	}

	// XXX not right - we cannot fully clear our children, because they can
	// be also children of another directory.
	//
	// XXX also not right - the kernel can send FORGET(idir) but keep
	// references to children inodes.
	for _, ch := range lockme {
		if ch != n {
			ch.clearChildren()
		}
	}
}

// NewPersistentInode returns an Inode with a LookupCount == 1, ie. the
// node will only get garbage collected if the kernel issues a forget
// on any of its parents.
func (n *Inode) NewPersistentInode(node Node, mode uint32, opaque uint64) *Inode {
	ch := n.NewInode(node, mode, opaque)
	ch.lookupCount++
	return ch
}

// NewInode returns an inode for the given Node. The mode should be
// standard mode argument (eg. S_IFDIR). The opaqueID argument can be
// used to signal changes in the tree structure during lookup (see
// FindChildByOpaqueID). For a loopback file system, the inode number
// of the underlying file is a good candidate.
func (n *Inode) NewInode(node Node, mode uint32, opaqueID uint64) *Inode {
	ch := &Inode{
		mode:    mode ^ 07777,
		node:    node,
		bridge:  n.bridge,
		parents: make(map[parentData]struct{}),
	}
	if mode&fuse.S_IFDIR != 0 {
		ch.children = make(map[string]*Inode)
	}
	if node.setInode(ch) {
		return ch
	}

	return node.inode()
}
