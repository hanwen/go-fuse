package fuse

import (
	"fmt"
	"sync"
)

// The inode reflects the kernel's idea of the inode.
type inode struct {
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
	fsInode     FsNode
	
	children    map[string]*inode

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
}

func (me *inode) createChild(name string, isDir bool, fsi FsNode, conn *FileSystemConnector) *inode {
	me.treeLock.Lock()
	defer me.treeLock.Unlock()

	ch := me.children[name]
	if ch != nil {
		panic(fmt.Sprintf("already have a child at %v %q", me.nodeId, name))
	}
	ch = conn.newInode(isDir)
	ch.fsInode = fsi
 	fsi.SetInode(ch)
	ch.mount = me.mount
	ch.treeLock = me.treeLock
	ch.lookupCount = 1
	
	me.addChild(name, ch)
	return ch
}

func (me *inode) getChild(name string) (child *inode) {
	me.treeLock.Lock()
	defer me.treeLock.Unlock()
	
	return me.children[name]
}

// Must be called with treeLock for the mount held.
func (me *inode) addChild(name string, child *inode) {
	if paranoia {
		ch := me.children[name]
		if ch != nil {
			panic(fmt.Sprintf("Already have an inode with same name: %v: %v", name, ch))
		}
	}

	me.children[name] = child
	me.fsInode.AddChild(name, child.fsInode)
}

// Must be called with treeLock for the mount held.
func (me *inode) rmChild(name string) (ch *inode) {
	ch = me.children[name]
	if ch != nil {
		me.children[name] = nil, false
		me.fsInode.RmChild(name, ch.fsInode)
	}
	return ch
}

// Can only be called on untouched inodes.
func (me *inode) mountFs(fs *inodeFs, opts *FileSystemOptions) {
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
func (me *inode) canUnmount() bool {
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

func (me *inode) IsDir() bool {
	return me.children != nil
}

func (me *inode) getMountDirEntries() (out []DirEntry) {
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

// Returns any open file, preferably a r/w one.
func (me *inode) getAnyFile() (file File) {
	me.openFilesMutex.Lock()
	defer me.openFilesMutex.Unlock()
	
	for _, f := range me.openFiles {
		if file == nil || f.OpenFlags & O_ANYWRITE != 0 {
			file = f.file
		}
	}
	return file
}

// Returns an open writable file for the given inode.
func (me *inode) getWritableFiles() (files []File) {
	me.openFilesMutex.Lock()
	defer me.openFilesMutex.Unlock()

	for _, f := range me.openFiles {
		if f.OpenFlags & O_ANYWRITE != 0 {
			files = append(files, f.file)
		}
	}
	return files
}

const initDirSize = 20

func (me *inode) verify(cur *fileSystemMount) {
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
