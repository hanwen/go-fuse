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

func (me *subFsInfo) getGlobalNode(node uint64) (uint64, bool) {
	me.ParentNodeIdsLock.RLock()
	defer me.ParentNodeIdsLock.RUnlock()
	global, ok := me.ParentNodeIds[node]
	return global, ok
}

func (me *subFsInfo) dropGlobalNode(node uint64) {
	me.ParentNodeIdsLock.Lock()
	defer me.ParentNodeIdsLock.Unlock()
	me.ParentNodeIds[node] = 0, false
}

func (me *subFsInfo) addGlobalNode(local uint64, global uint64) {
	me.ParentNodeIdsLock.Lock()
	defer me.ParentNodeIdsLock.Unlock()
	me.ParentNodeIds[local] = global
}


////////////////////////////////////////////////////////////////

type subInodeData struct {
	SubFs *subFsInfo

	// NodeId in the sub filesystem.
	NodeId uint64

	LookupCount int
}

func (me *subInodeData) Deletable() bool {
	return me.LookupCount <= 0 && (me.NodeId != fuse.FUSE_ROOT_ID || me.SubFs == nil)
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

	fuse.DefaultRawFuseFileSystem
}

type SubmountFileSystemOptions struct {
	fuse.TimeoutOptions
}

////////////////
// Routines that do locking.

func (me *SubmountFileSystem) registerLookup(subInode uint64, subfs *subFsInfo) (globalNodeId uint64) {
	globalNodeId, ok := subfs.getGlobalNode(subInode)

	var globalNode *subInodeData = nil

	me.nodeMapLock.Lock()
	defer me.nodeMapLock.Unlock()
	if ok {
		globalNode = me.nodeMap[globalNodeId]
	} else {
		globalNodeId = me.nextFreeInode
		me.nextFreeInode++

		globalNode = &subInodeData{
			SubFs:  subfs,
			NodeId: subInode,
		}

		me.nodeMap[globalNodeId] = globalNode
		subfs.addGlobalNode(subInode, globalNodeId)
	}
	globalNode.LookupCount++
	return globalNodeId
}

func (me *SubmountFileSystem) getSubFs(local uint64) (nodeid uint64, subfs *subFsInfo) {
	me.nodeMapLock.RLock()
	defer me.nodeMapLock.RUnlock()
	data, ok := me.nodeMap[local]
	if !ok {
		return 0, nil
	}

	return data.NodeId, data.SubFs
}

func (me *SubmountFileSystem) addFileSystem(name string, fs fuse.RawFileSystem, attr fuse.Attr) bool {
	me.toplevelEntriesLock.Lock()
	defer me.toplevelEntriesLock.Unlock()
	_, ok := me.toplevelEntries[name]
	if ok {
		return false
	}

	subfs := &subFsInfo{
		Name: name,
		Fs:   fs,
		Attr: attr,
	}

	me.toplevelEntries[name] = subfs

	me.nodeMapLock.Lock()
	defer me.nodeMapLock.Unlock()

	me.nodeMap[me.nextFreeInode] = &subInodeData{
		SubFs:       subfs,
		NodeId:      fuse.FUSE_ROOT_ID,
		LookupCount: 0,
	}
	subfs.ParentNodeIds = map[uint64]uint64{fuse.FUSE_ROOT_ID: me.nextFreeInode}
	subfs.GlobalNodeId = me.nextFreeInode

	subfs.Attr.Mode |= fuse.S_IFDIR
	subfs.Attr.Ino = me.nextFreeInode

	me.nextFreeInode++

	return true
}

func (me *SubmountFileSystem) removeFileSystem(name string) *subFsInfo {
	me.toplevelEntriesLock.Lock()
	defer me.toplevelEntriesLock.Unlock()

	subfs, ok := me.toplevelEntries[name]
	if !ok {
		return nil
	}

	me.toplevelEntries[name] = nil, false

	// We leave the keys of node map as is, since the kernel may
	// still issue requests with nodeids in it.
	me.nodeMapLock.Lock()
	defer me.nodeMapLock.Unlock()

	for _, v := range me.nodeMap {
		if v.SubFs == subfs {
			v.SubFs = nil
		}
	}

	return subfs
}

func (me *SubmountFileSystem) listFileSystems() ([]string, []uint32) {
	me.toplevelEntriesLock.RLock()
	defer me.toplevelEntriesLock.RUnlock()

	names := make([]string, len(me.toplevelEntries))
	modes := make([]uint32, len(me.toplevelEntries))

	j := 0
	for name, entry := range me.toplevelEntries {
		names[j] = name
		modes[j] = entry.Attr.Mode
		j++
	}
	return names, modes
}

func (me *SubmountFileSystem) lookupRoot(name string) (out *fuse.EntryOut, code fuse.Status) {
	me.toplevelEntriesLock.RLock()
	subfs, ok := me.toplevelEntries[name]
	me.toplevelEntriesLock.RUnlock()

	if !ok {
		out = new(fuse.EntryOut)
		out.NodeId = 0
		fuse.SplitNs(me.Options.NegativeTimeout, &out.EntryValid, &out.EntryValidNsec)
		return nil, fuse.ENOENT
	}

	me.nodeMapLock.RLock()
	dentry, ok := me.nodeMap[subfs.GlobalNodeId]
	me.nodeMapLock.RUnlock()

	if !ok {
		panic(fmt.Sprintf("unknown toplevel node %d", subfs.GlobalNodeId))
	}

	dentry.LookupCount++

	out = new(fuse.EntryOut)
	out.NodeId = subfs.GlobalNodeId
	out.Attr = subfs.Attr

	fuse.SplitNs(me.Options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	fuse.SplitNs(me.Options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

	return out, fuse.OK
}

func (me *SubmountFileSystem) forget(h *fuse.InHeader, input *fuse.ForgetIn) *subInodeData {
	me.nodeMapLock.Lock()
	defer me.nodeMapLock.Unlock()

	subNodeData := me.nodeMap[h.NodeId]
	globalNodeId := h.NodeId

	subNodeData.LookupCount -= int(input.Nlookup)
	if subNodeData.Deletable() {
		me.nodeMap[globalNodeId] = nil, false
		if subNodeData.SubFs != nil {
			subNodeData.SubFs.dropGlobalNode(subNodeData.NodeId)
		}
	}
	return subNodeData
}

////////////////////////////////////////////////////////////////
// Functions below should not need locking primitives.

// Caller should init after this returns successfully.
func (me *SubmountFileSystem) AddFileSystem(name string, fs fuse.RawFileSystem, attr fuse.Attr) bool {
	ok := me.addFileSystem(name, fs, attr)
	return ok
}

// Caller should call destroy.
func (me *SubmountFileSystem) RemoveFileSystem(name string) fuse.RawFileSystem {
	subfs := me.removeFileSystem(name)
	if subfs != nil {
		return subfs.Fs
	}
	return nil
}

func (me *SubmountFileSystem) Lookup(h *fuse.InHeader, name string) (out *fuse.EntryOut, code fuse.Status) {
	if h.NodeId == fuse.FUSE_ROOT_ID {
		return me.lookupRoot(name)
	}

	subInode, subfs := me.getSubFs(h.NodeId)
	if subInode == 0 {
		panic("parent unknown")
	}

	h.NodeId = subInode
	out, code = subfs.Fs.Lookup(h, name)

	// TODO - is there a named constant for 0 ?
	if out == nil || out.NodeId == 0 {
		return out, code
	}

	out.NodeId = me.registerLookup(out.NodeId, subfs)
	return out, code
}

func (me *SubmountFileSystem) Forget(h *fuse.InHeader, input *fuse.ForgetIn) {
	if h.NodeId != fuse.FUSE_ROOT_ID {
		subNodeData := me.forget(h, input)
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
func (me *SubmountFileSystem) Init(h *fuse.InHeader, input *fuse.InitIn) (*fuse.InitOut, fuse.Status) {
	return new(fuse.InitOut), fuse.OK
}

func (me *SubmountFileSystem) Destroy(h *fuse.InHeader, input *fuse.InitIn) {
	for _, v := range me.toplevelEntries {
		v.Fs.Destroy(h, input)
	}
}


func (me *SubmountFileSystem) GetAttr(header *fuse.InHeader, input *fuse.GetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		out := new(fuse.AttrOut)
		// TODO - what to answer for this?
		out.Attr.Mode = fuse.S_IFDIR | 0755
		return out, fuse.OK
	}
	subId, subfs := me.getSubFs(header.NodeId)
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

func (me *SubmountFileSystem) Open(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseFile, status fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, fuse.ENOENT
	}

	return subfs.Fs.Open(header, input)
}

func (me *SubmountFileSystem) SetAttr(header *fuse.InHeader, input *fuse.SetAttrIn) (out *fuse.AttrOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.SetAttr(header, input)
	if out != nil {
		out.Attr.Ino, _ = subfs.getGlobalNode(out.Ino)
	}
	return out, code
}

func (me *SubmountFileSystem) Readlink(header *fuse.InHeader) (out []byte, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	return subfs.Fs.Readlink(header)
}

func (me *SubmountFileSystem) Mknod(header *fuse.InHeader, input *fuse.MknodIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Mknod(header, input, name)

	if out != nil {
		out.NodeId = me.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (me *SubmountFileSystem) Mkdir(header *fuse.InHeader, input *fuse.MkdirIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Mkdir(header, input, name)

	if out != nil {
		out.NodeId = me.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (me *SubmountFileSystem) Unlink(header *fuse.InHeader, name string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.Unlink(header, name)
}

func (me *SubmountFileSystem) Rmdir(header *fuse.InHeader, name string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}
	return subfs.Fs.Rmdir(header, name)
}

func (me *SubmountFileSystem) Symlink(header *fuse.InHeader, pointedTo string, linkName string) (out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Symlink(header, pointedTo, linkName)
	if out != nil {
		out.NodeId = me.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (me *SubmountFileSystem) Rename(header *fuse.InHeader, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID || input.Newdir == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.Rename(header, input, oldName, newName)
}

func (me *SubmountFileSystem) Link(header *fuse.InHeader, input *fuse.LinkIn, name string) (out *fuse.EntryOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	out, code = subfs.Fs.Link(header, input, name)
	if out != nil {
		out.NodeId = me.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}
	return out, code
}

func (me *SubmountFileSystem) SetXAttr(header *fuse.InHeader, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}

	return subfs.Fs.SetXAttr(header, input, attr, data)
}

func (me *SubmountFileSystem) GetXAttr(header *fuse.InHeader, attr string) (data []byte, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	return subfs.Fs.GetXAttr(header, attr)
}

func (me *SubmountFileSystem) RemoveXAttr(header *fuse.InHeader, attr string) (code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return fuse.ENOENT
	}
	return subfs.Fs.RemoveXAttr(header, attr)
}

func (me *SubmountFileSystem) Access(header *fuse.InHeader, input *fuse.AccessIn) (code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	return subfs.Fs.Access(header, input)
}

func (me *SubmountFileSystem) Create(header *fuse.InHeader, input *fuse.CreateIn, name string) (flags uint32, fuseFile fuse.RawFuseFile, out *fuse.EntryOut, code fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		// ENOSYS ?
		return 0, nil, nil, fuse.EPERM
	}

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, nil, fuse.ENOENT
	}
	flags, fuseFile, out, code = subfs.Fs.Create(header, input, name)
	if out != nil {
		out.NodeId = me.registerLookup(out.NodeId, subfs)
		out.Attr.Ino = out.NodeId
	}

	return flags, fuseFile, out, code
}

func (me *SubmountFileSystem) Bmap(header *fuse.InHeader, input *fuse.BmapIn) (out *fuse.BmapOut, code fuse.Status) {
	var subfs *subFsInfo
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	return subfs.Fs.Bmap(header, input)
}

func (me *SubmountFileSystem) Ioctl(header *fuse.InHeader, input *fuse.IoctlIn) (out *fuse.IoctlOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}
	return subfs.Fs.Ioctl(header, input)
}

func (me *SubmountFileSystem) Poll(header *fuse.InHeader, input *fuse.PollIn) (out *fuse.PollOut, code fuse.Status) {
	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return nil, fuse.ENOENT
	}

	return subfs.Fs.Poll(header, input)
}

func (me *SubmountFileSystem) OpenDir(header *fuse.InHeader, input *fuse.OpenIn) (flags uint32, fuseFile fuse.RawFuseDir, status fuse.Status) {
	if header.NodeId == fuse.FUSE_ROOT_ID {
		return 0, NewSubmountFileSystemTopDir(me), fuse.OK
	}

	// TODO - we have to parse and unparse the readdir results, to substitute inodes.

	var subfs *subFsInfo
	header.NodeId, subfs = me.getSubFs(header.NodeId)
	if subfs == nil {
		return 0, nil, fuse.ENOENT
	}
	return subfs.Fs.OpenDir(header, input)
}

func (me *SubmountFileSystem) Release(header *fuse.InHeader, f fuse.RawFuseFile) {
	// TODO - should run release on subfs too.
}

func (me *SubmountFileSystem) ReleaseDir(header *fuse.InHeader, f fuse.RawFuseDir) {
	// TODO - should run releasedir on subfs too.
}


////////////////////////////////////////////////////////////////

type SubmountFileSystemTopDir struct {
	names    []string
	modes    []uint32
	nextRead int

	fuse.DefaultRawFuseDir
}

func NewSubmountFileSystemTopDir(fs *SubmountFileSystem) *SubmountFileSystemTopDir {
	out := new(SubmountFileSystemTopDir)

	out.names, out.modes = fs.listFileSystems()
	return out
}

func (me *SubmountFileSystemTopDir) ReadDir(input *fuse.ReadIn) (*fuse.DirEntryList, fuse.Status) {
	de := fuse.NewDirEntryList(int(input.Size))

	for me.nextRead < len(me.names) {
		i := me.nextRead
		if de.AddString(me.names[i], fuse.FUSE_UNKNOWN_INO, me.modes[i]) {
			me.nextRead++
		} else {
			break
		}
	}
	return de, fuse.OK
}
