package fuse

import (
	"fmt"
	"log"
	"sync"
)
var _ = log.Println

// The inode reflects the kernel's idea of the inode.
type Inode struct {
	handled Handled

	// Constant during lifetime.
	nodeId uint64

	// Number of open files and its protection.
	openFilesMutex sync.Mutex
	openFiles      []*openedFile

	// treeLock is a pointer to me.mount.treeLock; we need store
	// this mutex separately, since unmount may set me.mount = nil
	// during Unmount().  Constant during lifetime.
	//
	// If multiple treeLocks must be acquired, the treeLocks
	// closer to the root must be acquired first.
	treeLock *sync.RWMutex

	// All data below is protected by treeLock.
	fsInode FsNode

	children map[string]*Inode

	// Contains directories that function as mounts. The entries
	// are duplicated in children.
	mounts      map[string]*fileSystemMount
	lookupCount int

	// Non-nil if this is a mountpoint.
	mountPoint *fileSystemMount

	// The file system to which this node belongs.  Is constant
	// during the lifetime, except upon Unmount() when it is set
	// to nil.
	mount *fileSystemMount

	connector *FileSystemConnector
}

// public methods.

// LockTree() Locks the mutex used for tree operations, and returns the
// unlock function.
func (me *Inode) LockTree() func() {
	// TODO - this API is tricky.
	me.treeLock.Lock()
	return func() { me.treeLock.Unlock() }
}

// Returns any open file, preferably a r/w one.
func (me *Inode) AnyFile() (file File) {
	me.openFilesMutex.Lock()
	defer me.openFilesMutex.Unlock()

	for _, f := range me.openFiles {
		if file == nil || f.WithFlags.OpenFlags&O_ANYWRITE != 0 {
			file = f.WithFlags.File
		}
	}
	return file
}

func (me *Inode) Children() (out map[string]*Inode) {
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	out = map[string]*Inode{}
	for k, v := range me.children {
		out[k] = v
	}
	return out
}

func (me *Inode) FsNode() FsNode {
	return me.fsInode
}

// Files() returns an opens file that have bits in common with the
// give mask.  Use mask==0 to return all files.
func (me *Inode) Files(mask uint32) (files []WithFlags) {
	me.openFilesMutex.Lock()
	defer me.openFilesMutex.Unlock()
	log.Println("inod.files:", me.openFiles)
	for _, f := range me.openFiles {
		if mask == 0 || f.WithFlags.OpenFlags&mask != 0 {
			files = append(files, f.WithFlags)
		}
	}
	return files
}

func (me *Inode) IsDir() bool {
	return me.children != nil
}

// Creates an Inode as child. 
func (me *Inode) CreateChild(name string, isDir bool, fsi FsNode) *Inode {
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	ch := me.children[name]
	if ch != nil {
		panic(fmt.Sprintf("already have a child at %v %q", me.nodeId, name))
	}
	ch = me.connector.newInode(isDir)
	ch.fsInode = fsi
	fsi.SetInode(ch)
	ch.mount = me.mount
	ch.treeLock = me.treeLock
	ch.lookupCount = 1
	ch.connector = me.connector

	me.addChild(name, ch)
	return ch
}

func (me *Inode) GetChild(name string) (child *Inode) {
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	return me.children[name]
}

// Must be called with treeLock for the mount held.
func (me *Inode) addChild(name string, child *Inode) {
	if paranoia {
		ch := me.children[name]
		if ch != nil {
			panic(fmt.Sprintf("Already have an Inode with same name: %v: %v", name, ch))
		}
	}
	me.children[name] = child

	if child.mountPoint == nil {
		me.fsInode.AddChild(name, child.fsInode)
	}
}

// Must be called with treeLock for the mount held.
func (me *Inode) rmChild(name string) (ch *Inode) {
	ch = me.children[name]
	if ch != nil {
		me.children[name] = nil, false
		me.fsInode.RmChild(name, ch.fsInode)
	}
	return ch
}

// Can only be called on untouched inodes.
func (me *Inode) mountFs(fs NodeFileSystem, opts *FileSystemOptions) {
	me.mountPoint = &fileSystemMount{
		fs:         fs,
		openFiles:  NewHandleMap(true),
		mountInode: me,
		options:    opts,
	}
	me.mount = me.mountPoint
	me.treeLock = &me.mountPoint.treeLock
	me.fsInode = fs.Root()
	me.fsInode.SetInode(me)
}

// Must be called with treeLock held.
func (me *Inode) canUnmount() bool {
	for _, v := range me.children {
		if v.mountPoint != nil {
			// This access may be out of date, but it is no
			// problem to err on the safe side.
			return false
		}
		if !v.canUnmount() {
			return false
		}
	}

	me.openFilesMutex.Lock()
	defer me.openFilesMutex.Unlock()
	return len(me.openFiles) == 0
}

func (me *Inode) getMountDirEntries() (out []DirEntry) {
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	for k, _ := range me.mounts {
		out = append(out, DirEntry{
			Name: k,
			Mode: S_IFDIR,
		})
	}
	return out
}

const initDirSize = 20

func (me *Inode) verify(cur *fileSystemMount) {
	if me.mountPoint != nil {
		if me != me.mountPoint.mountInode {
			panic("mountpoint mismatch")
		}
		cur = me.mountPoint
	}
	if me.mount != cur {
		panic(fmt.Sprintf("me.mount not set correctly %v %v", me.mount, cur))
	}

	for name, m := range me.mounts {
		if m.mountInode != me.children[name] {
			panic(fmt.Sprintf("mountpoint parent mismatch: node:%v name:%v ch:%v",
				me.mountPoint, name, me.children))
		}
	}

	for _, ch := range me.children {
		if ch == nil {
			panic("Found nil child.")
		}
		ch.verify(cur)
	}
}
