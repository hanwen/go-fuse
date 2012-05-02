package fuse

import (
	"log"
	"sync"
	"unsafe"
	
	"github.com/hanwen/go-fuse/raw"
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
func (m *fileSystemMount) mountName() string {
	for k, v := range m.parentInode.mounts {
		if m == v {
			return k
		}
	}
	panic("not found")
	return ""
}

func (m *fileSystemMount) setOwner(attr *raw.Attr) {
	if m.options.Owner != nil {
		attr.Owner = *(*raw.Owner)(m.options.Owner)
	}
}

func (m *fileSystemMount) attrToEntry(attr *raw.Attr) (out *raw.EntryOut) {
	out = &raw.EntryOut{}
	out.Attr = *attr

	splitDuration(m.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	splitDuration(m.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	m.setOwner(&out.Attr)
	if attr.Mode & S_IFDIR == 0 && attr.Nlink == 0 {
		out.Nlink = 1
	}
	return out
}

func (m *fileSystemMount) fillAttr(a *raw.Attr, nodeId uint64) (out *raw.AttrOut) {
	out = &raw.AttrOut{}
	out.Attr = *a
	splitDuration(m.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	m.setOwner(&out.Attr)
	out.Ino = nodeId
	return out
}

func (m *fileSystemMount) getOpenedFile(h uint64) *openedFile {
	b := (*openedFile)(unsafe.Pointer(m.openFiles.Decode(h)))
	if m.connector.Debug && b.WithFlags.Description != "" {
		log.Printf("File %d = %q", h, b.WithFlags.Description)
	}
	return b
}

func (m *fileSystemMount) unregisterFileHandle(handle uint64, node *Inode) *openedFile {
	obj := m.openFiles.Forget(handle)
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

func (m *fileSystemMount) registerFileHandle(node *Inode, dir rawDir, f File, flags uint32) (uint64, *openedFile) {
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
	handle := m.openFiles.Register(&b.Handled, b)
	return handle, b
}

// Creates a return entry for a non-existent path.
func (m *fileSystemMount) negativeEntry() (*raw.EntryOut, Status) {
	if m.options.NegativeTimeout > 0.0 {
		out := new(raw.EntryOut)
		out.NodeId = 0
		splitDuration(m.options.NegativeTimeout, &out.EntryValid, &out.EntryValidNsec)
		return out, OK
	}
	return nil, ENOENT
}
