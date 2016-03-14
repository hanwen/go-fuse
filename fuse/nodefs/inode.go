package nodefs

import (
	"log"
	"sync"

	"github.com/hanwen/go-fuse/fuse"
)

// An Inode reflects the kernel's idea of the inode.  Inodes have IDs
// that are communicated to the kernel, and they have a tree
// structure: a directory Inode may contain named children.  Each
// Inode object is paired with a Node object, which file system
// implementers should supply.
type Inode struct {
	handled handled

	// Generation number of the inode. Each (re)use of an inode
	// should have a unique generation number.
	generation uint64

	// Number of open files and its protection.
	openFilesMutex sync.Mutex
	openFiles      []*openedFile

	fsInode Node

	// Each inode belongs to exactly one fileSystemMount. This
	// pointer is constant during the lifetime, except upon
	// Unmount() when it is set to nil.
	mount *fileSystemMount

	// All data below is protected by treeLock.
	// Children of this inode, indexed by file name.
	children map[string]*Inode

	// Due to hard links, an Inode may have many parents. We must
	// keep track of them to remove ourselves from all of our parents on
	// FORGET.
	parents []*Inode

	// Non-nil if this inode is a mountpoint, ie. the Root of a
	// NodeFileSystem.
	mountPoint *fileSystemMount
}

func newInode(isDir bool, fsNode Node) *Inode {
	me := new(Inode)
	if isDir {
		me.children = make(map[string]*Inode, initDirSize)
	}
	me.fsInode = fsNode
	me.fsInode.SetInode(me)
	return me
}

// public methods.

// Returns any open file, preferably a r/w one.
func (n *Inode) AnyFile() (file File) {
	n.openFilesMutex.Lock()
	for _, f := range n.openFiles {
		if file == nil || f.WithFlags.OpenFlags&fuse.O_ANYWRITE != 0 {
			file = f.WithFlags.File
		}
	}
	n.openFilesMutex.Unlock()

	return file
}

// Children returns all children of this inode.
func (n *Inode) Children() (out map[string]*Inode) {
	n.mount.treeLock.RLock()
	out = make(map[string]*Inode, len(n.children))
	for k, v := range n.children {
		out[k] = v
	}
	n.mount.treeLock.RUnlock()

	return out
}

// FsChildren returns all the children from the same filesystem.  It
// will skip mountpoints.
func (n *Inode) FsChildren() (out map[string]*Inode) {
	n.mount.treeLock.RLock()
	out = map[string]*Inode{}
	for k, v := range n.children {
		if v.mount == n.mount {
			out[k] = v
		}
	}
	n.mount.treeLock.RUnlock()

	return out
}

// Node returns the file-system specific node.
func (n *Inode) Node() Node {
	return n.fsInode
}

// Files() returns an opens file that have bits in common with the
// give mask.  Use mask==0 to return all files.
func (n *Inode) Files(mask uint32) (files []WithFlags) {
	n.openFilesMutex.Lock()
	for _, f := range n.openFiles {
		if mask == 0 || f.WithFlags.OpenFlags&mask != 0 {
			files = append(files, f.WithFlags)
		}
	}
	n.openFilesMutex.Unlock()
	return files
}

// IsDir returns true if this is a directory.
func (n *Inode) IsDir() bool {
	return n.children != nil
}

// NewChild adds a new child inode to this inode.
func (n *Inode) NewChild(name string, isDir bool, fsi Node) *Inode {
	ch := newInode(isDir, fsi)
	ch.mount = n.mount
	ch.parents = []*Inode{}
	n.AddChild(name, ch)
	return ch
}

// GetChild returns a child inode with the given name, or nil if it
// does not exist.
func (n *Inode) GetChild(name string) (child *Inode) {
	n.mount.treeLock.RLock()
	child = n.children[name]
	n.mount.treeLock.RUnlock()

	return child
}

// AddChild adds a child inode. The parent inode must be a directory
// node.
func (n *Inode) AddChild(name string, child *Inode) {
	if child == nil {
		log.Panicf("adding nil child as %q", name)
	}
	n.mount.treeLock.Lock()
	n.addChild(name, child)
	n.mount.treeLock.Unlock()
}

// RmChild removes an inode by name, and returns it. It returns nil if
// child does not exist.
func (n *Inode) RmChild(name string) (ch *Inode) {
	n.mount.treeLock.Lock()
	ch = n.rmChild(name)
	n.mount.treeLock.Unlock()
	return
}

//////////////////////////////////////////////////////////////
// private

// addChild - Add "child" to our children under name "name".
// Also adds ourselves to the list of the child's parents.
// Must be called with treeLock for the mount held.
func (n *Inode) addChild(name string, child *Inode) {
	ch := n.children[name]
	if ch != nil {
		log.Printf("Already have an Inode with same name: %v: %v", name, ch)
		if paranoia {
			log.Panic("paranoia: panic on duplicate inode")
		}
	}
	// Add child to our children
	n.children[name] = child

	// Add ourselves the the child's parents
	child.parents = append(child.parents, n)
}

// rmChildByRef - Drop child "ref" from our list of children.
// The Inode may be in the list multiple times under different names.
// Every entry is removed but only the last one is returned.
//
// Must be called with treeLock for the mount held.
func (n *Inode) rmChildByRef(ref *Inode) (ch *Inode) {
	for name, ino := range(n.children) {
		if ino == ref {
			ch = n.rmChild(name)
		}
	}
	return ch
}

// rmChild - Drop "name" from our children and remove ourself as their parent.
// Must be called with treeLock for the mount held.
func (n *Inode) rmChild(name string) (ch *Inode) {
	ch = n.children[name]
	if ch != nil {
		// Drop "name" from our children
		delete(n.children, name)

		// Remove ourselves from the child's parents
		idx := -1
		for i, v := range(ch.parents) {
			if v == n {
				idx = i
				break
			}
		}
		if idx < 0 {
			panic("idx<0")
		}
		// Delete the "idx" entry from the middle of the slice by moving the
		// last element over it and truncating the slice
		ch.parents[idx] = ch.parents[len(ch.parents)-1]
		ch.parents = ch.parents[:len(ch.parents)-1]
	}
	return ch
}

// Can only be called on untouched root inodes.
func (n *Inode) mountFs(opts *Options) {
	n.mountPoint = &fileSystemMount{
		openFiles:  newPortableHandleMap(),
		mountInode: n,
		options:    opts,
	}
	n.mount = n.mountPoint
}

// Must be called with treeLock held.
func (n *Inode) canUnmount() bool {
	for _, v := range n.children {
		if v.mountPoint != nil {
			// This access may be out of date, but it is no
			// problem to err on the safe side.
			return false
		}
		if !v.canUnmount() {
			return false
		}
	}

	n.openFilesMutex.Lock()
	ok := len(n.openFiles) == 0
	n.openFilesMutex.Unlock()
	return ok
}

func (n *Inode) getMountDirEntries() (out []fuse.DirEntry) {
	n.mount.treeLock.RLock()
	for k, v := range n.children {
		if v.mountPoint != nil {
			out = append(out, fuse.DirEntry{
				Name: k,
				Mode: fuse.S_IFDIR,
			})
		}
	}
	n.mount.treeLock.RUnlock()

	return out
}

const initDirSize = 20

func (n *Inode) verify(cur *fileSystemMount) {
	n.handled.verify()
	if n.mountPoint != nil {
		if n != n.mountPoint.mountInode {
			log.Panicf("mountpoint mismatch %v %v", n, n.mountPoint.mountInode)
		}
		cur = n.mountPoint

		cur.treeLock.Lock()
		defer cur.treeLock.Unlock()
	}
	if n.mount != cur {
		log.Panicf("n.mount not set correctly %v %v", n.mount, cur)
	}

	for nm, ch := range n.children {
		if ch == nil {
			log.Panicf("Found nil child: %q", nm)
		}
		ch.verify(cur)
	}
}
