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
	node   *pathInode
	paths []clientInodePath
}

// Stores the inode<->path map and provides safe operations on the map
type clientInodeContainer struct {
	entries map[uint64]*clientInodeEntry
	lock sync.Mutex
}

func NewClientInodeContainer() (c clientInodeContainer) {
	c.entries = map[uint64]*clientInodeEntry{}
	return
}

// Get node reference
func (c *clientInodeContainer) getNode(ino uint64) *pathInode {

	if ino == 0 {
		log.Panicf("clientinodes bug: getNode ino=0")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	entry := c.entries[ino]
	if entry != nil {
		return entry.node
	}
	return nil
}

// Add path to inode
func (c *clientInodeContainer) add(ino uint64, node *pathInode, name string, parent *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}

	if ino == 0 {
		log.Panicf("clientinodes bug: ino=0, name=%s", name)
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		entry = &clientInodeEntry{node: node}
		c.entries[ino] = entry
	}

	if entry.node != node {
		log.Panicf("clientinodes bug: add node reference mismatch, ino=%d, name=%s", ino, name)
	}

	for _, p := range entry.paths {
		if p.parent == parent && p.name == name {
			log.Panicf("clientinodes bug: duplicate entry, ino=%s, name=%s", ino, name)
		}
	}

	entry.paths = append(entry.paths, clientInodePath{parent: parent, name: name})

	if node.pathFs.debug {
		log.Printf("clientinodes: added ino=%d name=%s (%d hard links)", ino, name, len(entry.paths))
	}
}

// Remove path from inode. Drops the inode entry if this is the last path.
func (c *clientInodeContainer) rm(ino uint64, node *pathInode, name string, parent *pathInode) bool {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return true
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		log.Panicf("clientinodes bug: rm: inode %d name %s has no entry", ino, name)
	}
	if entry.node != node {
		log.Panicf("clientinodes bug: rm: inode %d name %s node reference mismatch", ino, name)
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
		panic("clientinodes bug: rm: path not found")
	}
	if node.pathFs.debug {
		log.Printf("clientinodes: removed ino=%d name=%s (%d hard links remaining)", ino, name, len(p)-1)
	}
	// The last hard link for this inode is being deleted. Drop the entry completely.
	// Note: We do this AFTER checking "idx < 0" to catch inconsistencies.
	if len(p) == 1 {
		delete(c.entries, ino)
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
		node.Parent = p[0].parent
		node.Name = p[0].name
	}
	return false
}


// Completely drop the inode with all its paths
func (c *clientInodeContainer) drop(ino uint64, node *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}

	if ino == 0 {
		log.Panicf("clientinodes bug: drop ino=0, name=%s", node.Name)
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.entries, ino)
}

// Verify that we have "node" stored for "ino". Panic if not.
func (c *clientInodeContainer) verify(ino uint64, node *pathInode) {
	if !node.pathFs.options.ClientInodes || ino == InoIgnore {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	entry := c.entries[ino]
	if entry == nil {
		log.Panicf("clientinodes bug: verify: ino %d not found, Name='%s'", ino, node.Name)
	}

	if entry.node != node {
		log.Panicf("clientinodes bug: verify: ino %d node mismatch, node=%p, entry.node=%p", ino, node, entry.node)
	}
}
