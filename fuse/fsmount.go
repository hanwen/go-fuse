package fuse

import (
	"log"
	"os"
	"sync"
	"unsafe"
)

var _ = log.Println

// openedFile stores either an open dir or an open file.
type openedFile struct {
	Handled

	WithFlags

	dir rawDir
}

type fileSystemMount struct {
	// The file system we mounted here.
	fs NodeFileSystem

	// Node that we were mounted on.
	mountInode *Inode

	// Parent to the mountInode.
	parentInode *Inode

	// Options for the mount.
	options *FileSystemOptions

	// Protects Children hashmaps within the mount.  treeLock
	// should be acquired before openFilesLock.
	treeLock sync.RWMutex

	// Manage filehandles of open files.
	openFiles HandleMap

	Debug bool

	connector *FileSystemConnector
}

// Must called with lock for parent held.
func (me *fileSystemMount) mountName() string {
	for k, v := range me.parentInode.mounts {
		if me == v {
			return k
		}
	}
	panic("not found")
	return ""
}

func (me *fileSystemMount) setOwner(attr *Attr) {
	if me.options.Owner != nil {
		attr.Owner = *me.options.Owner
	}
}

func (me *fileSystemMount) fileInfoToEntry(fi *os.FileInfo) (out *EntryOut) {
	out = &EntryOut{}
	splitNs(me.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	splitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	CopyFileInfo(fi, &out.Attr)
	me.setOwner(&out.Attr)
	if !fi.IsDirectory() && fi.Nlink == 0 {
		out.Nlink = 1
	}
	return out
}

func (me *fileSystemMount) fileInfoToAttr(fi *os.FileInfo, nodeId uint64) (out *AttrOut) {
	out = &AttrOut{}
	CopyFileInfo(fi, &out.Attr)
	splitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	me.setOwner(&out.Attr)
	out.Ino = nodeId
	return out
}

func (me *fileSystemMount) getOpenedFile(h uint64) *openedFile {
	b := (*openedFile)(unsafe.Pointer(me.openFiles.Decode(h)))
	if me.connector.Debug && b.WithFlags.Description != "" {
		log.Printf("File %d = %q", h, b.WithFlags.Description)
	}
	return b
}

func (me *fileSystemMount) unregisterFileHandle(handle uint64, node *Inode) *openedFile {
	obj := me.openFiles.Forget(handle)
	opened := (*openedFile)(unsafe.Pointer(obj))
	node.openFilesMutex.Lock()
	defer node.openFilesMutex.Unlock()

	idx := -1
	for i, v := range node.openFiles {
		if v == opened {
			idx = i
			break
		}
	}

	l := len(node.openFiles)
	node.openFiles[idx] = node.openFiles[l-1]
	node.openFiles = node.openFiles[:l-1]

	return opened
}

func (me *fileSystemMount) registerFileHandle(node *Inode, dir rawDir, f File, flags uint32) (uint64, *openedFile) {
	node.openFilesMutex.Lock()
	defer node.openFilesMutex.Unlock()
	b := &openedFile{
		dir: dir,
		WithFlags: WithFlags{
			File:      f,
			OpenFlags: flags,
		},
	}

	for {
		withFlags, ok := f.(*WithFlags)
		if !ok {
			break
		}

		b.WithFlags.File = withFlags.File
		b.WithFlags.FuseFlags |= withFlags.FuseFlags
		b.WithFlags.Description += withFlags.Description
		f = withFlags.File
	}

	if b.WithFlags.File != nil {
		b.WithFlags.File.SetInode(node)
	}
	node.openFiles = append(node.openFiles, b)
	handle := me.openFiles.Register(&b.Handled, b)
	return handle, b
}

// Creates a return entry for a non-existent path.
func (me *fileSystemMount) negativeEntry() (*EntryOut, Status) {
	if me.options.NegativeTimeout > 0.0 {
		out := new(EntryOut)
		out.NodeId = 0
		splitNs(me.options.NegativeTimeout, &out.EntryValid, &out.EntryValidNsec)
		return out, OK
	}
	return nil, ENOENT
}
