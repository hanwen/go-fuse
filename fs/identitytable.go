// Copyright 2026 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"log"
	"runtime/debug"
	"sync"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// nodeEntry is the per-nodeid state the kernel-facing tables need
// beyond the *Inode itself: which file handles are open on it, and (if
// passthrough is in use) its backing-fd state.
//
// A nodeEntry lives for exactly as long as its nodeid is registered:
// mapIdentityTable.forget deletes it in the same step that drops the
// nodeid. There is nothing extra to clean up at that point because the
// FUSE protocol guarantees openFiles is already empty and the backing
// fields are already zero - a node with open handles or an active
// backing-fd registration keeps its lookup count (and therefore its
// entry) alive.
type nodeEntry struct {
	inode *Inode

	// file handles open on this node.
	openFiles []uint32

	// backing-fd (passthrough) state. Protected by rawBridge.backingMu,
	// not by mapIdentityTable.mu: addBackingID/releaseBackingIDRef
	// (bridge.go) hold a lock across a call into ServerCallbacks to
	// register/unregister the backing fd, and mu - used for identity
	// resolution across the whole filesystem - must not be held during
	// that call. Callers reach these fields via the *nodeEntry returned
	// by the identity lookups below.
	backingID         int32
	backingIDRefcount int
}

// mapIdentityTable owns every kernel-facing number: nodeids and file
// handles, and the mappings back to the entries they identify. It is
// held by value as rawBridge.ids (see bridge.go).
//
// All methods lock internally (via mu) and return with no locks held.
// Every method may be called while the caller already holds one or
// more Inode.mu locks (via lockNodes/lockNode2) or a fileEntry.mu, but
// never the reverse: no method here calls back into code that acquires
// Inode.mu or fileEntry.mu while mu is held. This keeps mu innermost,
// consistent with the package's existing rule that "locks for inodes
// must be taken before rawBridge's locks".
type mapIdentityTable struct {
	mu sync.Mutex

	// stableAttrs is used to detect already-known nodes and hard links
	// by looking at:
	// 1) file type ......... StableAttr.Mode
	// 2) inode number ...... StableAttr.Ino
	// 3) generation number . StableAttr.Gen
	stableAttrs  map[StableAttr]*Inode
	automaticIno uint64

	// The *Node ID* is an arbitrary uint64 identifier chosen by the FUSE
	// library. It is used to identify *nodes* (files/directories/symlinks/...)
	// in the communication between the FUSE library and the Linux kernel.
	//
	// nodes translates between the NodeID and its nodeEntry. A simple
	// incrementing counter is used as the NodeID (see nextNodeId).
	nodes      map[uint64]*nodeEntry
	nextNodeId uint64

	// nodeCountHigh records the highest number of entries we had in the
	// nodes map. As the size of stableAttrs tracks nodes (+- a few
	// entries due to concurrent FORGETs, LOOKUPs, and the fixed NodeID
	// 1), this is also a good estimate for stableAttrs.
	nodeCountHigh int

	files []*fileEntry
	// indices of files that are not allocated.
	freeFiles []uint32
}

// initIdentityTable prepares a zero-value mapIdentityTable for use.
// The pointer receiver ensures the embedding rawBridge - and its
// mutex - is never copied.
func (t *mapIdentityTable) initIdentityTable(firstAutomaticIno uint64) {
	t.automaticIno = firstAutomaticIno
	if t.automaticIno == 0 {
		t.automaticIno = 1 << 63
	}
	t.stableAttrs = make(map[StableAttr]*Inode)
	t.nodes = make(map[uint64]*nodeEntry)
	// Fh 0 means no file handle.
	t.files = []*fileEntry{{}}
}

func (t *mapIdentityTable) registerRoot(root *Inode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes[1] = &nodeEntry{inode: root}
	t.nextNodeId = 2 // the root node has nodeid 1
}

func (t *mapIdentityTable) nextAutomaticIno() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	ino := t.automaticIno
	t.automaticIno++
	return ino
}

func (t *mapIdentityTable) allocateNodeID() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := t.nextNodeId
	t.nextNodeId++
	return id
}

func (t *mapIdentityTable) findByAttr(id StableAttr) *Inode {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stableAttrs[id]
}

// registerNew atomically checks for an existing node under id and, if
// none is registered yet (or it is already child), registers child and
// returns it along with its nodeEntry. If a different node is already
// registered under id, it is returned unchanged (with its own entry)
// and child is not registered. If exclusive is true, the existing-node
// check is skipped and child is unconditionally (re-)registered.
func (t *mapIdentityTable) registerNew(id StableAttr, child *Inode, exclusive bool) (*Inode, *nodeEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !exclusive {
		if old := t.stableAttrs[id]; old != nil && old != child {
			return old, t.nodes[old.nodeId]
		}
	}

	e := t.nodes[child.nodeId]
	if e == nil {
		// Fresh node: create its entry. If child was already
		// registered (the old == child case above), its entry - and
		// any accumulated openFiles/backing state - is left alone.
		e = &nodeEntry{inode: child}
		t.nodes[child.nodeId] = e
		if len(t.nodes) > t.nodeCountHigh {
			t.nodeCountHigh = len(t.nodes)
		}
	}
	// Any node that might be there is overwritten - it is obsolete now.
	t.stableAttrs[id] = child
	return child, e
}

// node resolves a (nodeid, fh) pair as sent by the kernel. fh may be 0,
// in which case the returned *fileEntry is the always-present
// placeholder entry for "no file handle".
func (t *mapIdentityTable) node(nodeID uint64, fh uint64) (*nodeEntry, *fileEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.nodes[nodeID], t.files[fh]
}

func (t *mapIdentityTable) forget(n *Inode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Dropping the node from stableAttrs guarantees that no new
	// references to this node are handed out to the kernel, hence we
	// can also safely delete it from nodes.
	delete(t.stableAttrs, n.stableAttr)
	delete(t.nodes, n.nodeId)
}

// compact tries to free memory that was previously used by forgotten
// nodes.
//
// Maps do not free all memory when elements get deleted
// ( https://github.com/golang/go/issues/20135 ).
// As a workaround, we recreate our two big maps (stableAttrs & nodes)
// every time they have shrunk dramatically (100 x smaller).
// In this case, `nodeCountHigh` is reset to the new (smaller) size.
func (t *mapIdentityTable) compact() {
	t.mu.Lock()

	if t.nodeCountHigh <= len(t.nodes)*100 {
		t.mu.Unlock()
		return
	}

	tmpStableAttrs := make(map[StableAttr]*Inode, len(t.stableAttrs))
	for i, v := range t.stableAttrs {
		tmpStableAttrs[i] = v
	}
	t.stableAttrs = tmpStableAttrs

	tmpNodes := make(map[uint64]*nodeEntry, len(t.nodes))
	for i, v := range t.nodes {
		tmpNodes[i] = v
	}
	t.nodes = tmpNodes

	t.nodeCountHigh = len(t.nodes)

	t.mu.Unlock()

	// Run outside t.mu
	debug.FreeOSMemory()
}

// registerFile hands out a fresh file handle number for f, open on the
// node behind e. flags are the open flags (eg. syscall.O_EXCL).
func (t *mapIdentityTable) registerFile(e *nodeEntry, f FileHandle, flags uint32) *fileEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	fe := &fileEntry{}
	if len(t.freeFiles) > 0 {
		last := len(t.freeFiles) - 1
		fe.fh = t.freeFiles[last]
		t.freeFiles = t.freeFiles[:last]
		t.files[fe.fh] = fe
	} else {
		fe.fh = uint32(len(t.files))
		t.files = append(t.files, fe)
	}

	if _, ok := f.(FileReaddirenter); ok {
		fe.lastRead = make([]fuse.DirEntry, 0, 100)
	}
	fe.nodeIndex = len(e.openFiles)
	fe.file = f
	fe.inode = e.inode
	e.openFiles = append(e.openFiles, fe.fh)

	return fe
}

// firstOpenFile returns any currently-open FileHandle recorded on e,
// plus a func that must be called once the caller is done using it.
// Used to fake a file handle for kernel requests that don't supply one
// (eg. GETATTR without an explicit fh).
func (t *mapIdentityTable) firstOpenFile(e *nodeEntry) (FileHandle, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, fh := range e.openFiles {
		fe := t.files[fh]
		fe.wg.Add(1)
		return fe.file, fe.wg.Done
	}
	return nil, func() {}
}

// detachFile unlinks the file handle fh from its owning node's entry
// and returns that entry and the fileEntry. The fh number itself does
// not become reusable until recycleFile is called - the caller may
// still need to run release callbacks against the fileEntry first.
func (t *mapIdentityTable) detachFile(nodeID uint64, fh uint64) (*nodeEntry, *fileEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	e := t.nodes[nodeID]
	if e == nil {
		log.Panicf("detachFile: unknown node %d", nodeID)
	}
	var entry *fileEntry
	if fh > 0 {
		last := len(e.openFiles) - 1
		entry = t.files[fh]
		if last != entry.nodeIndex {
			e.openFiles[entry.nodeIndex] = e.openFiles[last]
			t.files[e.openFiles[entry.nodeIndex]].nodeIndex = entry.nodeIndex
		}
		e.openFiles = e.openFiles[:last]
	}
	return e, entry
}

// recycleFile marks fh as free for reuse by a future registerFile call.
func (t *mapIdentityTable) recycleFile(fh uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.freeFiles = append(t.freeFiles, fh)
}
