package examplelib

import (
	"github.com/hanwen/go-fuse/fuse"
	"sync"
	"fmt"
)

var _ = fmt.Println

type subFsInfo struct {
	// Entry within global FS.
	Name string
	Fs   fuse.RawFileSystem

	// Inode in global namespace.
	GlobalNodeId uint64

	// Maps Fs's Inodes back to the parent inode.
	ParentNodeIds map[uint64]uint64

	// This must always be the inner lock in the locking order.
	ParentNodeIdsLock sync.RWMutex

	Attr fuse.Attr
}

func (self *subFsInfo) getGlobalNode(node uint64) (uint64, bool) {
	self.ParentNodeIdsLock.RLock()
	defer self.ParentNodeIdsLock.RUnlock()
	global, ok := self.ParentNodeIds[node]
	return global, ok
}

func (self *subFsInfo) dropGlobalNode(node uint64) {
	self.ParentNodeIdsLock.Lock()
	defer self.ParentNodeIdsLock.Unlock()
	self.ParentNodeIds[node] = 0, false
}

func (self *subFsInfo) addGlobalNode(local uint64, global uint64) {
	self.ParentNodeIdsLock.Lock()
	defer self.ParentNodeIdsLock.Unlock()
	self.ParentNodeIds[local] = global
}


////////////////////////////////////////////////////////////////

type subInodeData struct {
	SubFs *subFsInfo

	// NodeId in the sub filesystem.
	NodeId uint64

	LookupCount int
}

func (self *subInodeData) Deletable() bool {
	return self.LookupCount <= 0 && (self.NodeId != fuse.FUSE_ROOT_ID || self.SubFs == nil)
}

////////////////////////////////////////////////////////////////

// This is a file system that will composite multiple FUSE
// filesystems, so you can have for example
//
//  /dir1   -> one simple FUSE filesystem
//  /dir2   -> another simple FUSE filesystem
//  /config -> a FS interface to daemon configuration
//
// Each of these directories should be RawFileSystem instances.  This
// class takes care of the mapping between global inode ids and inode ids
// of each sub-FUSE filesystem.
// 
// No files or directories may be created in the toplevel dir. Instead,
// the daemon should issue AddFileSystem() and RemoveFileSystem()
// methods internally.  This could be done in response to writes in a
// /config directory.
type SubmountFileSystem struct {
	toplevelEntriesLock sync.RWMutex
	toplevelEntries     map[string]*subFsInfo

	// Mutex protects map and nextFreeInode.
	nodeMapLock   sync.RWMutex
	nodeMap       map[uint64]*subInodeData
	nextFreeInode uint64

	Options SubmountFileSystemOptions
}

type SubmountFileSystemOptions struct {
	fuse.TimeoutOptions
}

////////////////
// Routines that do locking. 

func (self *SubmountFileSystem) registerLookup(subInode uint64, subfs *subFsInfo) (globalNodeId uint64) {
	globalNodeId, ok := subfs.getGlobalNode(subInode)

	var globalNode *subInodeData = nil

	self.nodeMapLock.Lock()
	defer self.nodeMapLock.Unlock()
	if ok {
		globalNode = self.nodeMap[globalNodeId]
	} else {
		globalNodeId = self.nextFreeInode
		self.nextFreeInode++

		globalNode = &subInodeData{
			SubFs:  subfs,
			NodeId: subInode,
		}

		self.nodeMap[globalNodeId] = globalNode
		subfs.addGlobalNode(subInode, globalNodeId)
	}
	globalNode.LookupCount++
	return globalNodeId
}

func (self *SubmountFileSystem) getSubFs(local uint64) (nodeid uint64, subfs *subFsInfo) {
	self.nodeMapLock.RLock()
	defer self.nodeMapLock.RUnlock()
	data := self.nodeMap[local]
	return data.NodeId, data.SubFs
}

func (self *SubmountFileSystem) addFileSystem(name string, fs fuse.RawFileSystem, attr fuse.Attr) bool {
	self.toplevelEntriesLock.Lock()
	defer self.toplevelEntriesLock.Unlock()
	_, ok := self.toplevelEntries[name]
	if ok {
		return false
	}

	subfs := &subFsInfo{
		Name: name,
		Fs:   fs,
		Attr: attr,
	}

	self.toplevelEntries[name] = subfs

	self.nodeMapLock.Lock()
	defer self.nodeMapLock.Unlock()

	self.nodeMap[self.nextFreeInode] = &subInodeData{
		SubFs:       subfs,
		NodeId:      fuse.FUSE_ROOT_ID,
		LookupCount: 0,
	}
	subfs.ParentNodeIds = map[uint64]uint64{fuse.FUSE_ROOT_ID: self.nextFreeInode}
	subfs.GlobalNodeId = self.nextFreeInode

	subfs.Attr.Mode |= fuse.S_IFDIR
	subfs.Attr.Ino = self.nextFreeInode

	self.nextFreeInode++

	return true
}

func (self *SubmountFileSystem) removeFileSystem(name string) *subFsInfo {
	self.toplevelEntriesLock.Lock()
	defer self.toplevelEntriesLock.Unlock()

	subfs, ok := self.toplevelEntries[name]
	if !ok {
		return nil
	}

	self.toplevelEntries[name] = nil, false

	// We leave the keys of node map as is, since the kernel may
	// still issue requests with nodeids in it.
	self.nodeMapLock.Lock()
	defer self.nodeMapLock.Unlock()

	for _, v := range self.nodeMap {
		if v.SubFs == subfs {
			v.SubFs = nil
		}
	}

	return subfs
}

func (self *SubmountFileSystem) listFileSystems() ([]string, []uint32) {
	self.toplevelEntriesLock.RLock()
	defer self.toplevelEntriesLock.RUnlock()

	names := make([]string, len(self.toplevelEntries))
	modes := make([]uint32, len(self.toplevelEntries))

	j := 0
	for name, entry := range self.toplevelEntries {
		names[j] = name
		modes[j] = entry.Attr.Mode
		j++
	}
	return names, modes
}

func (self *SubmountFileSystem) lookupRoot(name string) (out *fuse.EntryOut, code fuse.Status) {
	self.toplevelEntriesLock.RLock()
	subfs, ok := self.toplevelEntries[name]
	self.toplevelEntriesLock.RUnlock()

	if !ok {
		out = new(fuse.EntryOut)
		out.NodeId = 0
		fuse.SplitNs(self.Options.NegativeTimeout, &out.EntryValid, &out.EntryValidNsec)
		return nil, fuse.ENOENT
	}

	self.nodeMapLock.RLock()
	dentry, ok := self.nodeMap[subfs.GlobalNodeId]
	self.nodeMapLock.RUnlock()

	if !ok {
		panic(fmt.Sprintf("unknown toplevel node %d", subfs.GlobalNodeId))
	}

	dentry.LookupCount++

	out = new(fuse.EntryOut)
	out.NodeId = subfs.GlobalNodeId
	out.Attr = subfs.Attr

	fuse.SplitNs(self.Options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	fuse.SplitNs(self.Options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

	return out, fuse.OK
}

func (self *SubmountFileSystem) forget(h *fuse.InHeader, input *fuse.ForgetIn) *subInodeData {
	self.nodeMapLock.Lock()
	defer self.nodeMapLock.Unlock()

	subNodeData := self.nodeMap[h.NodeId]
	globalNodeId := h.NodeId

	subNodeData.LookupCount -= int(input.Nlookup)
	if subNodeData.Deletable() {
		self.nodeMap[globalNodeId] = nil, false
		if subNodeData.SubFs != nil {
			subNodeData.SubFs.dropGlobalNode(subNodeData.NodeId)
		}
	}
	return subNodeData
}


////////////////////////////////////////////////////////////////
// Functions below should not need locking primitives. 


// Caller should init after this returns successfully.
func (self *SubmountFileSystem) AddFileSystem(name string, fs fuse.RawFileSystem, attr fuse.Attr) bool {
	ok := self.addFileSystem(name, fs, attr)
	return ok
}

// Caller should call destroy.
func (self *SubmountFileSystem) RemoveFileSystem(name string) fuse.RawFileSystem {
	subfs := self.removeFileSystem(name)
	if subfs != nil {
		return subfs.Fs
	}
	return nil
}

func (self *SubmountFileSystem) Lookup(h *fuse.InHeader, name string) (out *fuse.EntryOut, code fuse.Status) {
	if h.NodeId == fuse.FUSE_ROOT_ID {
		return self.lookupRoot(name)
	}

	subInode, subfs := self.getSubFs(h.NodeId)
	if subInode == 0 {
		panic("parent unknown")
	}

	h.NodeId = subInode
	out, code = subfs.Fs.Lookup(h, name)

	// TODO - is there a named constant for 0 ?
	if out == nil || out.NodeId == 0 {
		return out, code
	}

	out.NodeId = self.registerLookup(out.NodeId, subfs)
	return out, code
}

func (self *SubmountFileSystem) Forget(h *fuse.InHeader, input *fuse.ForgetIn) {
	if h.NodeId != fuse.FUSE_ROOT_ID {
		subNodeData := self.forget(h, input)
		if subNodeData != nil && subNodeData.SubFs != nil {
			h.NodeId = subNodeData.NodeId
			subNodeData.SubFs.Fs.Forget(h, input)
		}
	}
}

func NewSubmountFileSystem() *SubmountFileSystem {
	out := new(SubmountFileSystem)
	out.nextFreeInode = fuse.FUSE_ROOT_ID + 1
	out.nodeMap = make(map[uint64]*subInodeData)
	out.toplevelEntries = make(map[string]*subFsInfo)
	out.Options.TimeoutOptions = fuse.MakeTimeoutOptions()
	return out
}

// What to do about sub fs init's ?
func (self *SubmountFileSystem) Init(h *fuse.InHeader, input *fuse.InitIn) (*fuse.InitOut, fuse.Status) {
	return new(fuse.InitOut), fuse.OK
}

func (self *SubmountFileSystem) Destroy(h *fuse.InHeader, input *fuse.InitIn) {
	for _, v := range self.toplevelEntries {
		v.Fs.Destroy(h, input)
	}
}


func (self *SubmountFileSystem) GetAttr(header *fuse.InHeader, input *fuse.GetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		out := new(fuse.AttrOut)
		// TODO - what to answer for this?
		out.Attr.Mode = fuse.S_IFDIR | 0755
		return out, fuse.OK
	}
	subId, subfs := self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	// Looking for attributes of the sub-filesystem mountpoint.
	if header.NodeId == subfs.GlobalNodeId {
		out := new(fuse.AttrOut)
		out.Attr = subfs.Attr
		return out, fuse.OK
	}

	header.NodeId = subId

	out, code = subfs.Fs.GetAttr(header, input)
	if out != nil {
		out.Attr.Ino, _ = subfs.getGlobalNode(out.Ino)
	}
	return out, code
}

func (self *SubmountFileSystem) Open(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseFile, status fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, fuse.ENOENT
	}

	return subfs.Fs.Open(header, input)
}

func (self *SubmountFileSystem) SetAttr(header *fuse.InHeader, input *fuse.SetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.SetAttr(header, input)
	if out != nil {
		out.Attr.Ino, _ = subfs.getGlobalNode(out.Ino)
	}
	return out, code
}

func (self *SubmountFileSystem) Readlink(header *fuse.InHeader) (out []byte, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	return subfs.Fs.Readlink(header)
}

func (self *SubmountFileSystem) Mknod(header *fuse.InHeader, input *fuse.MknodIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Mknod(header, input, name)

	if out != nil {
		out.NodeId = self.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (self *SubmountFileSystem) Mkdir(header *fuse.InHeader, input *fuse.MkdirIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Mkdir(header, input, name)

	if out != nil {
		out.NodeId = self.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (self *SubmountFileSystem) Unlink(header *fuse.InHeader, name string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.Unlink(header, name)
}

func (self *SubmountFileSystem) Rmdir(header *fuse.InHeader, name string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}
	return subfs.Fs.Rmdir(header, name)
}

func (self *SubmountFileSystem) Symlink(header *fuse.InHeader, pointedTo string, linkName string) (out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Symlink(header, pointedTo, linkName)
	if out != nil {
		out.NodeId = self.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (self *SubmountFileSystem) Rename(header *fuse.InHeader, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID || input.Newdir == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.Rename(header, input, oldName, newName)
}

func (self *SubmountFileSystem) Link(header *fuse.InHeader, input *fuse.LinkIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Link(header, input, name)
	if out != nil {
		out.NodeId = self.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (self *SubmountFileSystem) SetXAttr(header *fuse.InHeader, input *fuse.SetXAttrIn) fuse.Status {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.SetXAttr(header, input)
}

func (self *SubmountFileSystem) GetXAttr(header *fuse.InHeader, input *fuse.GetXAttrIn) (out *fuse.GetXAttrOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	out, code = subfs.Fs.GetXAttr(header, input)
	return out, code
}

func (self *SubmountFileSystem) Access(header *fuse.InHeader, input *fuse.AccessIn) (code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	return subfs.Fs.Access(header, input)
}

func (self *SubmountFileSystem) Create(header *fuse.InHeader, input *fuse.CreateIn, name string) (flags uint32, fuseFile fuse.RawFuseFile, out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return 0, nil, nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, nil, fuse.ENOENT
	}
	flags, fuseFile, out, code = subfs.Fs.Create(header, input, name)
	if out != nil {
		out.NodeId = self.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}

	return flags, fuseFile, out, code
}

func (self *SubmountFileSystem) Bmap(header *fuse.InHeader, input *fuse.BmapIn) (out *fuse.BmapOut, code fuse.Status) {
	var subfs *subFsInfo
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	return subfs.Fs.Bmap(header, input)
}

func (self *SubmountFileSystem) Ioctl(header *fuse.InHeader, input *fuse.IoctlIn) (out *fuse.IoctlOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	return subfs.Fs.Ioctl(header, input)
}

func (self *SubmountFileSystem) Poll(header *fuse.InHeader, input *fuse.PollIn) (out *fuse.PollOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	return subfs.Fs.Poll(header, input)
}

func (self *SubmountFileSystem) OpenDir(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseDir, status fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		return 0, NewSubmountFileSystemTopDir(self), fuse.OK
	}

	// TODO - we have to parse and unparse the readdir results, to substitute inodes.

	var subfs *subFsInfo
	header.NodeId, subfs = self.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, fuse.ENOENT
	}
	return subfs.Fs.OpenDir(header, input)
}

func (self *SubmountFileSystem) Release(header *fuse.InHeader, f fuse.RawFuseFile) {
	// TODO - should run release on subfs too.
}

func (self *SubmountFileSystem) ReleaseDir(header *fuse.InHeader, f fuse.RawFuseDir) {
	// TODO - should run releasedir on subfs too.
}



////////////////////////////////////////////////////////////////

type SubmountFileSystemTopDir struct {
	names    []string
	modes    []uint32
	nextRead int
}

func NewSubmountFileSystemTopDir(fs *SubmountFileSystem) *SubmountFileSystemTopDir {
	out := new(SubmountFileSystemTopDir)

	out.names, out.modes = fs.listFileSystems()
	return out
}

func (self *SubmountFileSystemTopDir) ReadDir(input *fuse.ReadIn) (*fuse.DirEntryList, fuse.Status) {
	de := fuse.NewDirEntryList(int(input.Size))

	for self.nextRead < len(self.names) {
		i := self.nextRead
		if de.AddString(self.names[i], fuse.FUSE_UNKNOWN_INO, self.modes[i]) {
			self.nextRead++
		} else {
			break
		}
	}
	return de, fuse.OK
}

func (self *SubmountFileSystemTopDir) ReleaseDir() {

}

func (self *SubmountFileSystemTopDir) FsyncDir(input *fuse.FsyncIn) (code fuse.Status) {
	return fuse.ENOENT
}

