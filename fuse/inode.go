package fuse

import (
	"log"
	"sync"
)

var _ = log.Println

// The inode reflects the kernel's idea of the inode.  Inodes may be
// created automatically when the kernel does lookups inode, or by
// explicitly by calling Inode.New().
type Inode struct {
	handled Handled

	// Generation number of the inode. Each (re)use of an inode
	// should have a unique generation number.
	generation uint64

	// Number of open files and its protection.
	openFilesMutex sync.Mutex
	openFiles      []*openedFile

	fsInode FsNode

	// Each inode belongs to exactly one fileSystemMount. This
	// pointer is constant during the lifetime, except upon
	// Unmount() when it is set to nil.
	mount *fileSystemMount

	// treeLock is a pointer to me.mount.treeLock.  We store it
	// here for convenience.  Constant during lifetime of the
	// inode.
	//
	// If multiple treeLocks must be acquired, the treeLocks
	// closer to the root must be acquired first.
	treeLock *sync.RWMutex

	// All data below is protected by treeLock.
	children map[string]*Inode

	// Contains directories that function as mounts. The entries
	// are duplicated in children.
	mounts map[string]*fileSystemMount

	// The nodeId is only used to communicate to the kernel.  If
	// it is zero, it means the kernel does not know about this
	// Inode.  You should probably never read nodeId, but always
	// do lookupUpdate() on the node instead.
	nodeId uint64

	// lookupCount registers how often the kernel got this inode
	// back for a Lookup operation. This number is a reference
	// count, and the Forget operation lists how many references to drop.
	//
	// The lookupCount is exclusively used for managing the
	// lifetime of nodeId variable.  It is ok for a node to have 0
	// == lookupCount.  This can happen if the inode return false
	// for Deletable().
	lookupCount int

	// Non-nil if this inode is a mountpoint, ie. the Root of a
	// NodeFileSystem.
	mountPoint *fileSystemMount
}

func newInode(isDir bool, fsNode FsNode) *Inode {
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
		if file == nil || f.WithFlags.OpenFlags&O_ANYWRITE != 0 {
			file = f.WithFlags.File
		}
	}
	n.openFilesMutex.Unlock()

	return file
}

func (n *Inode) Children() (out map[string]*Inode) {
	n.treeLock.RLock()
	out = make(map[string]*Inode, len(n.children))
	for k, v := range n.children {
		out[k] = v
	}
	n.treeLock.RUnlock()

	return out
}

// FsChildren returns all the children from the same filesystem.  It
// will skip mountpoints.
func (n *Inode) FsChildren() (out map[string]*Inode) {
	n.treeLock.RLock()
	out = map[string]*Inode{}
	for k, v := range n.children {
		if v.mount == n.mount {
			out[k] = v
		}
	}
	n.treeLock.RUnlock()

	return out
}

func (n *Inode) FsNode() FsNode {
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

func (n *Inode) IsDir() bool {
	return n.children != nil
}

func (n *Inode) New(isDir bool, fsi FsNode) *Inode {
	ch := newInode(isDir, fsi)
	ch.mount = n.mount
	ch.treeLock = n.treeLock
	n.generation = ch.mount.connector.nextGeneration()
	return ch
}

func (n *Inode) GetChild(name string) (child *Inode) {
	n.treeLock.RLock()
	child = n.children[name]
	n.treeLock.RUnlock()

	return child
}

func (n *Inode) AddChild(name string, child *Inode) {
	if child == nil {
		log.Panicf("adding nil child as %q", name)
	}
	n.treeLock.Lock()
	n.addChild(name, child)
	n.treeLock.Unlock()
}

func (n *Inode) RmChild(name string) (ch *Inode) {
	n.treeLock.Lock()
	ch = n.rmChild(name)
	n.treeLock.Unlock()
	return
}

//////////////////////////////////////////////////////////////
// private

// Must be called with treeLock for the mount held.
func (n *Inode) addChild(name string, child *Inode) {
	if paranoia {
		ch := n.children[name]
		if ch != nil {
			log.Panicf("Already have an Inode with same name: %v: %v", name, ch)
		}
	}
	n.children[name] = child
}

// Must be called with treeLock for the mount held.
func (n *Inode) rmChild(name string) (ch *Inode) {
	ch = n.children[name]
	if ch != nil {
		delete(n.children, name)
	}
	return ch
}

// Can only be called on untouched inodes.
func (n *Inode) mountFs(fs NodeFileSystem, opts *FileSystemOptions) {
	n.mountPoint = &fileSystemMount{
		fs:         fs,
		openFiles:  NewHandleMap(false),
		mountInode: n,
		options:    opts,
	}
	n.mount = n.mountPoint
	n.treeLock = &n.mountPoint.treeLock
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

func (n *Inode) getMountDirEntries() (out []DirEntry) {
	n.treeLock.RLock()
	for k := range n.mounts {
		out = append(out, DirEntry{
			Name: k,
			Mode: S_IFDIR,
		})
	}
	n.treeLock.RUnlock()

	return out
}

const initDirSize = 20

func (n *Inode) verify(cur *fileSystemMount) {
	if n.lookupCount < 0 {
		log.Panicf("negative lookup count %d on node %d", n.lookupCount, n.nodeId)
	}
	if (n.lookupCount == 0) != (n.nodeId == 0) {
		log.Panicf("kernel registration mismatch: lookup %d id %d", n.lookupCount, n.nodeId)
	}
	if n.mountPoint != nil {
		if n != n.mountPoint.mountInode {
			log.Panicf("mountpoint mismatch %v %v", n, n.mountPoint.mountInode)
		}
		cur = n.mountPoint
	}
	if n.mount != cur {
		log.Panicf("n.mount not set correctly %v %v", n.mount, cur)
	}

	for name, m := range n.mounts {
		if m.mountInode != n.children[name] {
			log.Panicf("mountpoint parent mismatch: node:%v name:%v ch:%v",
				n.mountPoint, name, n.children)
		}
	}

	for nm, ch := range n.children {
		if ch == nil {
			log.Panicf("Found nil child: %q", nm)
		}
		ch.verify(cur)
	}
}
