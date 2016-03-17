package pathfs

// ClientInodes helpers (hard link tracking)

import (
	"log"
	"sync"
)

// This magic inode number (all ones) signals to us that any operation should
// pass as a no-op.
// This is used by unionfs.
const InoIgnore = ^uint64(0)

// An inode can have many paths (hard links!). This structure represents one
// hard link, characterised by parent directory and name.
type clientInodePath struct {
	parent *pathInode
	name   string
}

// An inode and its paths (hard links)
type clientInodeEntry struct {
	node  *pathInode
	paths []clientInodePath
}

// Stores the inode<->path map and provides safe operations on the map
type clientInodeContainer struct {
	entries map[uint64]*clientInodeEntry
	sync.Mutex
}

// NewClientInodeContainer - initializes the entries map and returns the
// container
func NewClientInodeContainer() (c clientInodeContainer) {
	c.entries = map[uint64]*clientInodeEntry{}
	return
}

// Get node reference
func (c *clientInodeContainer) getNode(ino uint64) *pathInode {

	if ino == 0 {
		log.Printf("clientinodes: bug: called getNode with ino=0")
		return nil
	}

	c.Lock()
	defer c.Unlock()

	entry := c.entries[ino]
	if entry != nil {
		return entry.node
	}
	return nil
}

// Add path to inode
func (c *clientInodeContainer) add(ino uint64, node *pathInode, newName string, newParent *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}
	if ino == 0 {
		log.Printf("clientinodes: bug: tried to add ino=0, name=%s", newName)
		return
	}
	if !newParent.Inode().IsDir() {
		log.Printf("clientinodes: bug: parent is not a directory")
		return
	}

	c.Lock()
	defer c.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		entry = &clientInodeEntry{node: node}
		c.entries[ino] = entry
	}

	// Consistency checks
	if entry.node != node {
		log.Printf("clientinodes: bug: add node reference mismatch, ino=%d, name=%s", ino, newName)
		return
	}
	for _, existingEntry := range entry.paths {
		// There can be more than one entry for the same parent,
		// but not with the same name. I.e. you cannot have two files with the
		// same name in one directory.
		existingParent := existingEntry.parent
		if existingParent == newParent && existingEntry.name == newName {
			log.Printf("clientinodes: bug: duplicate path entry, ino=%s, name=%s", ino, newName)
			return
		}

		// Two distinct parents can have the same grandparent, but only if they have different
		// names. I.e. you cannot have two directories with the same name in
		// one directory.
		existingGrandParent := existingParent.Parent
		newGrandParent := newParent.Parent
		if existingParent != newParent && existingGrandParent == newGrandParent && existingParent.Name == newParent.Name {
			log.Printf("clientinodes: bug: duplicate parents, existingParent=%p=%s, newParent=%p=%s",
				existingParent, existingParent.Name, newParent, newParent.Name)
			return
		}
	}

	entry.paths = append(entry.paths, clientInodePath{parent: newParent, name: newName})

	if node.pathFs.debug {
		log.Printf("clientinodes: added ino=%d name=%s (now has %d hard links)", ino, newName, len(entry.paths))
	}
}

// Remove path from inode. Drops the inode entry if this is the last path.
func (c *clientInodeContainer) rm(ino uint64, node *pathInode, name string, parent *pathInode) (dropped bool) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return true
	}

	c.Lock()
	defer c.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		log.Printf("clientinodes: bug: ino=%d name=%s has no entry", ino, name)
		return false
	}
	if entry.node != node {
		log.Printf("clientinodes: bug: ino=%d name=%s node reference mismatch, stored=%p passed=%p",
			ino, name, entry.node, node)
		return false
	}

	// Find the path that has us as the parent
	p := entry.paths
	idx := -1
	for i, v := range p {
		if v.parent == parent && v.name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		log.Printf("clientinodes: bug: path not found")
		return false
	}
	if node.pathFs.debug {
		log.Printf("clientinodes: removed ino=%d name=%s (%d hard links remaining)", ino, name, len(p)-1)
	}
	// The last hard link for this inode is being deleted. We still keep the
	// node reference because we want to be able to match files on LOOKUP
	// to the correct node. We only drop it on FORGET through drop().
	if len(p) == 1 {
		entry.paths = p[0:0]
		return true
	}
	// Delete the "idx" entry from the middle of the slice by moving the
	// last element over it and truncating the slice
	p[idx] = p[len(p)-1]
	p = p[:len(p)-1]
	entry.paths = p

	// If we have deleted the current primary parent,
	// reparent to a random remaining entry
	if node.Parent == parent && node.Name == name {
		newParent := p[0].parent
		newName := p[0].name
		// Our grandparent can only be nil if our parent is the root node
		if newParent.Parent == nil && newParent != node.pathFs.root {
			log.Printf("clientinodes: bug: attempted to reparent to a deleted node: %+v", newParent)
			return false
		}
		node.Parent = newParent
		node.Name = newName
	}
	return false
}

// Completely drop the inode with all its paths
func (c *clientInodeContainer) drop(ino uint64, node *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}
	if ino == 0 {
		log.Printf("clientinodes: bug: tried to drop ino=0, name=%s", node.Name)
		return
	}

	c.Lock()
	delete(c.entries, ino)
	c.Unlock()

	if node.pathFs.debug {
		log.Printf("clientinodes: dropped ino=%d", ino)
	}
}

// Verify that we have "node" stored for "ino". Panic if not.
func (c *clientInodeContainer) verify(ino uint64, node *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}

	c.Lock()
	defer c.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		log.Printf("clientinodes: bug: ino=%d not found, name=%s", ino, node.Name)
	}
	if entry.node != node {
		log.Printf("clientinodes: bug: ino=%d node mismatch, node=%p, entry.node=%p", ino, node, entry.node)
	}
}
