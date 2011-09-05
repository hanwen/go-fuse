package fuse

import (
	"fmt"
	"sync"
)

// The inode reflects the kernel's idea of the inode.
type inode struct {
	Handled

	// Constant during lifetime.
	NodeId uint64

	// Number of open files and its protection.
	OpenFilesMutex sync.Mutex
	OpenFiles      []*openedFile

	// treeLock is a pointer to me.mount.treeLock; we need store
	// this mutex separately, since unmount may set me.mount = nil
	// during Unmount().  Constant during lifetime.
	//
	// If multiple treeLocks must be acquired, the treeLocks
	// closer to the root must be acquired first.
	treeLock *sync.RWMutex

	// All data below is protected by treeLock.
	fsInode *fsInode
	
	Children    map[string]*inode

	// Contains directories that function as mounts. The entries
	// are duplicated in Children.
	Mounts      map[string]*fileSystemMount
	LookupCount int

	// Non-nil if this is a mountpoint.
	mountPoint *fileSystemMount

	// The file system to which this node belongs.  Is constant
	// during the lifetime, except upon Unmount() when it is set
	// to nil.
	mount *fileSystemMount
}

// Must be called with treeLock for the mount held.
func (me *inode) addChild(name string, child *inode) {
	if paranoia {
		ch := me.Children[name]
		if ch != nil {
			panic(fmt.Sprintf("Already have an inode with same name: %v: %v", name, ch))
		}
	}

	me.Children[name] = child
	me.fsInode.addChild(name, child.fsInode)
}

// Must be called with treeLock for the mount held.
func (me *inode) rmChild(name string) (ch *inode) {
	ch = me.Children[name]
	if ch != nil {
		me.Children[name] = nil, false
		me.fsInode.rmChild(name, ch.fsInode)
	}
	return ch
}

// Can only be called on untouched inodes.
func (me *inode) mountFs(fs FileSystem, opts *FileSystemOptions) {
	me.mountPoint = &fileSystemMount{
		fs:         fs,
		openFiles:  NewHandleMap(true),
		mountInode: me,
		options:    opts,
	}
	me.mount = me.mountPoint
	me.treeLock = &me.mountPoint.treeLock
}

// Must be called with treeLock held.
func (me *inode) canUnmount() bool {
	for _, v := range me.Children {
		if v.mountPoint != nil {
			// This access may be out of date, but it is no
			// problem to err on the safe side.
			return false
		}
		if !v.canUnmount() {
			return false
		}
	}

	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()
	return len(me.OpenFiles) == 0
}

func (me *inode) IsDir() bool {
	return me.Children != nil
}

func (me *inode) getMountDirEntries() (out []DirEntry) {
	me.treeLock.RLock()
	defer me.treeLock.RUnlock()

	for k, _ := range me.Mounts {
		out = append(out, DirEntry{
			Name: k,
			Mode: S_IFDIR,
		})
	}
	return out
}

// Returns any open file, preferably a r/w one.
func (me *inode) getAnyFile() (file File) {
	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()
	
	for _, f := range me.OpenFiles {
		if file == nil || f.OpenFlags & O_ANYWRITE != 0 {
			file = f.file
		}
	}
	return file
}

// Returns an open writable file for the given inode.
func (me *inode) getWritableFiles() (files []File) {
	me.OpenFilesMutex.Lock()
	defer me.OpenFilesMutex.Unlock()

	for _, f := range me.OpenFiles {
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

	for name, m := range me.Mounts {
		if m.mountInode != me.Children[name] {
			panic(fmt.Sprintf("mountpoint parent mismatch: node:%v name:%v ch:%v",
				me.mountPoint, name, me.Children))
		}
	}
	
	for _, ch := range me.Children {
		if ch == nil {
			panic("Found nil child.")
		}
		ch.verify(cur)
	}
}
