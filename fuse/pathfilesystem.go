package fuse

import (
	"bytes"
	"sync"
	"fmt"
	"log"
	"path"
	"strings"
)

// TODO should rename to dentry?
type inodeData struct {
	Parent      *inodeData
	NodeId      uint64
	Name        string
	LookupCount int

	Type     uint32 
	// Number of inodeData that have this as parent.
	RefCount int

	// If non-nil the file system mounted here.
	Mounted     PathFilesystem

	// If yes, we are looking to unmount the mounted fs.
	unmountPending bool
}

// Should implement some hash table method instead?
func inodeDataKey(parentInode uint64, name string) string {
	// TODO - use something more efficient than Sprintf.
	return fmt.Sprintf("%x:%s", parentInode, name)
}

func (self *inodeData) Key() string {
	var p uint64 = 0
	if self.Parent != nil {
		p = self.Parent.NodeId
	}
	return inodeDataKey(p, self.Name)
}

func (self *inodeData) GetPath() (path string, fs PathFilesystem) {
	// TODO - softcode this.
	var components [100]string

	j := len(components)
	inode := self
	for ; inode != nil && inode.Mounted == nil; inode = inode.Parent {
		j--
		components[j] = inode.Name
	}

	fullPath := strings.Join(components[j:], "/")
	if !inode.unmountPending {
		fs = inode.Mounted
	}
	return fullPath, fs
}

type TimeoutOptions struct {
	EntryTimeout    float64
	AttrTimeout     float64
	NegativeTimeout float64
}

func MakeTimeoutOptions() TimeoutOptions {
	return TimeoutOptions{
		NegativeTimeout: 0.0,
		AttrTimeout:     1.0,
		EntryTimeout:    1.0,
	}
}

type PathFileSystemConnectorOptions struct {
	TimeoutOptions
}

type PathFileSystemConnector struct {
	// Protects the hashmap, its contents and the nextFreeInode counter.
	lock sync.RWMutex

	// Invariants
	// - For all values, (RefCount > 0 || LookupCount > 0).
	// - For all values, value = inodePathMap[value.Key()]
	// - For all values, value = inodePathMapByInode[value.NodeId]

	// fuse.c seems to have different lifetimes for the different
	// hashtables, which could lead to the same directory entry
	// existing twice with different generated inode numbers, if
	// we have (FORGET, LOOKUP) on a directory entry with RefCount
	// > 0.
	inodePathMap        map[string]*inodeData
	inodePathMapByInode map[uint64]*inodeData
	nextFreeInode       uint64
	
	options PathFileSystemConnectorOptions
	Debug		    bool
}

// Must be called with lock held.
func (self *PathFileSystemConnector) setParent(data *inodeData, parentId uint64) {
	newParent := self.inodePathMapByInode[parentId]
	if data.Parent == newParent {
		return
	}

	if newParent == nil {
		panic("Unknown parent")
	}

	oldParent := data.Parent
	if oldParent != nil {
		self.unrefNode(oldParent)
	}
	data.Parent = newParent
	if newParent != nil {
		newParent.RefCount++
	}
}

// Must be called with lock held.
func (self *PathFileSystemConnector) unrefNode(data *inodeData) {
	data.RefCount--
	if data.RefCount <= 0 && data.LookupCount <= 0 {
		self.inodePathMapByInode[data.NodeId] = nil, false
	}
}

func (self *PathFileSystemConnector) lookupUpdate(nodeId uint64, name string) *inodeData {
	self.lock.Lock()
	defer self.lock.Unlock()

	key := inodeDataKey(nodeId, name)
	data, ok := self.inodePathMap[key]
	if !ok {
		data = new(inodeData)
		self.setParent(data, nodeId)
		data.NodeId = self.nextFreeInode
		data.Name = name
		self.nextFreeInode++

		self.inodePathMapByInode[data.NodeId] = data
		self.inodePathMap[key] = data
	}

	data.LookupCount++
	return data
}

func (self *PathFileSystemConnector) getInodeData(nodeid uint64) *inodeData {
	self.lock.RLock()
	defer self.lock.RUnlock()

	val := self.inodePathMapByInode[nodeid]
	if val == nil {
		panic(fmt.Sprintf("inode %v unknown", nodeid))
	}
	return val
}

func (self *PathFileSystemConnector) forgetUpdate(nodeId uint64, forgetCount int) {
	self.lock.Lock()
	defer self.lock.Unlock()

	data, ok := self.inodePathMapByInode[nodeId]
	if ok {
		data.LookupCount -= forgetCount
		if data.LookupCount <= 0 && data.RefCount <= 0 && (data.Mounted == nil || data.unmountPending) {
			self.inodePathMap[data.Key()] = nil, false
		}
	}
}

func (self *PathFileSystemConnector) renameUpdate(oldParent uint64, oldName string, newParent uint64, newName string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	oldKey := inodeDataKey(oldParent, oldName)
	data := self.inodePathMap[oldKey]
	if data == nil {
		// This can happen if a rename raced with an unlink or
		// another rename.
		//
		// TODO - does the VFS layer allow this?
		//
		// TODO - is this an error we should signal?
		return
	}

	self.inodePathMap[oldKey] = nil, false

	self.setParent(data, newParent)
	data.Name = newName
	newKey := data.Key()

	target := self.inodePathMap[newKey]
	if target != nil {
		// This could happen if some other thread creates a
		// file in the destination position.
		//
		// TODO - Does the VFS layer allow this?
		//
		// fuse.c just removes the node from its internal
		// tables, which might lead to paths being both directories
		// (parents) and normal files?
		self.inodePathMap[newKey] = nil, false

		self.setParent(target, FUSE_ROOT_ID)
		target.Name = fmt.Sprintf("overwrittenByRename%d", self.nextFreeInode)
		self.nextFreeInode++

		self.inodePathMap[target.Key()] = target
	}

	self.inodePathMap[data.Key()] = data
}

func (self *PathFileSystemConnector) unlinkUpdate(nodeid uint64, name string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	oldKey := inodeDataKey(nodeid, name)
	data := self.inodePathMap[oldKey]

	if data != nil {
		self.inodePathMap[oldKey] = nil, false
		self.unrefNode(data)
	}
}

// Walk the file system starting from the root.
func (self *PathFileSystemConnector) findInode(fullPath string) *inodeData {
	fullPath = strings.TrimLeft(path.Clean(fullPath), "/")
	comps := strings.Split(fullPath, "/", -1)
	
	self.lock.RLock()
	defer self.lock.RUnlock()

	node := self.inodePathMapByInode[FUSE_ROOT_ID]
	for i, component := range(comps) {
		if len(component) == 0 {
			continue
		}
		
		key := inodeDataKey(node.NodeId, component)
		node = self.inodePathMap[key]
		if node == nil {
			panic(fmt.Sprintf("findInode: %v %v", i, fullPath))
		}
	}
	return node
}

////////////////////////////////////////////////////////////////
// Below routines should not access inodePathMap(ByInode) directly,
// and there need no locking.

func NewPathFileSystemConnector(fs PathFilesystem) (out *PathFileSystemConnector) {
	out = new(PathFileSystemConnector)
	out.inodePathMap = make(map[string]*inodeData)
	out.inodePathMapByInode = make(map[uint64]*inodeData)

	rootData := new(inodeData)
	rootData.NodeId = FUSE_ROOT_ID
	rootData.Type = ModeToType(S_IFDIR)
	
	out.inodePathMap[rootData.Key()] = rootData
	out.inodePathMapByInode[FUSE_ROOT_ID] = rootData
	out.nextFreeInode = FUSE_ROOT_ID + 1

	out.options.NegativeTimeout = 0.0
	out.options.AttrTimeout = 1.0
	out.options.EntryTimeout = 1.0

	if code := out.Mount("/", fs); code != OK {
		panic("root mount failed.")
	}
	return out
}

func (self *PathFileSystemConnector) SetOptions(opts PathFileSystemConnectorOptions) {
	self.options = opts
}


func (self *PathFileSystemConnector) Mount(path string, fs PathFilesystem) Status {
	node := self.findInode(path)

	// TODO - check that fs was not mounted elsewhere.
	if node.RefCount > 0 {
		return EBUSY
	}

	if node.Type & ModeToType(S_IFDIR) == 0 {
		return EINVAL
	}
	
	code := fs.Mount(self)
	if code != OK {
		if self.Debug {
			log.Println("Mount error: ", path, code)
		}
		return code
	}
	
	if self.Debug {
		log.Println("Mount: ", fs, "on", path, node)
	}

	// TODO - this is technically a race-condition.
	node.Mounted = fs
	node.unmountPending = false

	return OK
}

func (self *PathFileSystemConnector) Unmount(path string) Status {
	node := self.findInode(path)
	if node == nil || node.Mounted == nil {
		panic(path)
	}
	// TODO - check if we have open files.

	if self.Debug {
		log.Println("Unmount: ", node)
	}
	// node manipulations are racy?
	if node.RefCount > 0 {
		node.Mounted.Unmount()
		node.unmountPending = true
	} else {
		node.Mounted = nil
	}
	return OK
}

func (self *PathFileSystemConnector) GetPath(nodeid uint64) (path string, fs PathFilesystem) {
	return self.getInodeData(nodeid).GetPath()
}

func (self *PathFileSystemConnector) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	// TODO ?
	return new(InitOut), OK
}

func (self *PathFileSystemConnector) Destroy(h *InHeader, input *InitIn) {
	// TODO - umount all.
}

func (self *PathFileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, status Status) {
	parent := self.getInodeData(header.NodeId)

	// TODO - fuse.c has special case code for name == "." and
	// "..", those lookups happen if FUSE_EXPORT_SUPPORT is set in
	// Init.
	fullPath, fs := parent.GetPath()
	if fs == nil {
		return NegativeEntry(self.options.NegativeTimeout), OK
	}
	fullPath = path.Join(fullPath, name)

	attr, err := fs.GetAttr(fullPath)
	
	if err == ENOENT && self.options.NegativeTimeout > 0.0 {
		return NegativeEntry(self.options.NegativeTimeout), OK
	}

	if err != OK {
		return nil, err
	}

	data := self.lookupUpdate(header.NodeId, name)
	data.Type = ModeToType(attr.Mode)
	
	out = new(EntryOut)
	out.NodeId = data.NodeId
	out.Generation = 1 // where to get the generation?

	SplitNs(self.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(self.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	out.Attr = *attr
	return out, OK
}

func (self *PathFileSystemConnector) Forget(h *InHeader, input *ForgetIn) {
	self.forgetUpdate(h.NodeId, int(input.Nlookup))
}

func (self *PathFileSystemConnector) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	// TODO - should we update inodeData.Type?
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}
	attr, err := fs.GetAttr(fullPath)
	if err != OK {
		return nil, err
	}

	out = new(AttrOut)
	out.Attr = *attr

	SplitNs(self.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

	return out, OK
}

func (self *PathFileSystemConnector) OpenDir(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseDir, status Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return 0, nil, ENOENT
	}
	// TODO - how to handle return flags, the FUSE open flags?
	f, err := fs.OpenDir(fullPath)
	if err != OK {
		return 0, nil, err
	}

	return 0, f, OK
}

func (self *PathFileSystemConnector) Open(header *InHeader, input *OpenIn) (flags uint32, fuseFile RawFuseFile, status Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return 0, nil, ENOENT
	}
	// TODO - how to handle return flags, the FUSE open flags?
	f, err := fs.Open(fullPath, input.Flags)
	if err != OK {
		return 0, nil, err
	}
	return 0, f, OK
}

func (self *PathFileSystemConnector) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	var err Status = OK

	// TODO - support Fh.   (FSetAttr/FGetAttr/FTruncate.)
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}

	if input.Valid&FATTR_MODE != 0 {
		err = fs.Chmod(fullPath, input.Mode)
	}
	if err != OK && (input.Valid&FATTR_UID != 0 || input.Valid&FATTR_GID != 0) {
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		err = fs.Chown(fullPath, uint32(input.Uid), uint32(input.Gid))
	}
	if input.Valid&FATTR_SIZE != 0 {
		fs.Truncate(fullPath, input.Size)
	}
	if err != OK && (input.Valid&FATTR_ATIME != 0 || input.Valid&FATTR_MTIME != 0) {
		err = fs.Utimens(fullPath,
			uint64(input.Atime*1e9)+uint64(input.Atimensec),
			uint64(input.Mtime*1e9)+uint64(input.Mtimensec))
	}
	if err != OK && (input.Valid&FATTR_ATIME_NOW != 0 || input.Valid&FATTR_MTIME_NOW != 0) {
		// TODO - should set time to now. Maybe just reuse
		// Utimens() ?  Go has no UTIME_NOW unfortunately.
	}
	if err != OK {
		return nil, err
	}

	// TODO - where to get GetAttrIn.Flags / Fh ?
	return self.GetAttr(header, new(GetAttrIn))
}

func (self *PathFileSystemConnector) Readlink(header *InHeader) (out []byte, code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}
	val, err := fs.Readlink(fullPath)
	return bytes.NewBufferString(val).Bytes(), err
}

func (self *PathFileSystemConnector) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}	
	fullPath = path.Join(fullPath, name)
	err := fs.Mknod(fullPath, input.Mode, uint32(input.Rdev))
	if err != OK {
		return nil, err
	}
	return self.Lookup(header, name)
}

func (self *PathFileSystemConnector) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}	
	err := fs.Mkdir(path.Join(fullPath, name), input.Mode)
	if err != OK {
		return nil, err
	}
	out, code = self.Lookup(header, name)
	return out, code
}

func (self *PathFileSystemConnector) Unlink(header *InHeader, name string) (code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return ENOENT
	}	
	code = fs.Unlink(path.Join(fullPath, name))

	// Like fuse.c, we update our internal tables.
	self.unlinkUpdate(header.NodeId, name)

	return code
}

func (self *PathFileSystemConnector) Rmdir(header *InHeader, name string) (code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return ENOENT
	}
	code = fs.Rmdir(path.Join(fullPath, name))
	self.unlinkUpdate(header.NodeId, name)
	return code
}

func (self *PathFileSystemConnector) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	fullPath, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return nil, ENOENT
	}	
	err := fs.Symlink(pointedTo, path.Join(fullPath, linkName))
	if err != OK {
		return nil, err
	}

	out, code = self.Lookup(header, linkName)
	return out, code
}

func (self *PathFileSystemConnector) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	oldPath, oldFs := self.GetPath(header.NodeId)
	newPath, fs := self.GetPath(input.Newdir)
	if fs == nil || oldFs == nil {
		return ENOENT
	}
	if fs != oldFs {
		return EXDEV
	}
	
	oldPath = path.Join(oldPath, oldName)
	newPath = path.Join(newPath, newName)
	code = fs.Rename(oldPath, newPath)
	if code != OK {
		return
	}

	// It is conceivable that the kernel module will issue a
	// forget for the old entry, and a lookup request for the new
	// one, but the fuse.c updates its client-side tables on its
	// own, so we do this as well.
	//
	// It should not hurt for us to do it here as well, although
	// it remains unclear how we should update Count.
	self.renameUpdate(header.NodeId, oldName, input.Newdir, newName)
	return code
}

func (self *PathFileSystemConnector) Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status) {
	orig, fs := self.GetPath(input.Oldnodeid)
	newName, newFs := self.GetPath(header.NodeId)

	if fs == nil || newFs == nil {
		return nil, ENOENT
	}
	
	if newFs != fs {
		return nil, EXDEV
	}
	newName = path.Join(newName, filename)
	err := fs.Link(orig, newName)

	if err != OK {
		return nil, err
	}

	return self.Lookup(header, filename)
}

func (self *PathFileSystemConnector) Access(header *InHeader, input *AccessIn) (code Status) {
	p, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return ENOENT
	}	
	return fs.Access(p, input.Mask)
}

func (self *PathFileSystemConnector) Create(header *InHeader, input *CreateIn, name string) (flags uint32, fuseFile RawFuseFile, out *EntryOut, code Status) {
	directory, fs := self.GetPath(header.NodeId)
	if fs == nil {
		return 0, nil, nil, ENOENT
	}
	fullPath := path.Join(directory, name)

	f, err := fs.Create(fullPath, uint32(input.Flags), input.Mode)
	if err != OK {
		return 0, nil, nil, err
	}

	out, code = self.Lookup(header, name)
	return 0, f, out, code
}

////////////////////////////////////////////////////////////////
// unimplemented.

func (self *PathFileSystemConnector) SetXAttr(header *InHeader, input *SetXAttrIn) Status {
	return ENOSYS
}

func (self *PathFileSystemConnector) GetXAttr(header *InHeader, input *GetXAttrIn) (out *GetXAttrOut, code Status) {
	return nil, ENOSYS
}

func (self *PathFileSystemConnector) Bmap(header *InHeader, input *BmapIn) (out *BmapOut, code Status) {
	return nil, ENOSYS
}

func (self *PathFileSystemConnector) Ioctl(header *InHeader, input *IoctlIn) (out *IoctlOut, code Status) {
	return nil, ENOSYS
}

func (self *PathFileSystemConnector) Poll(header *InHeader, input *PollIn) (out *PollOut, code Status) {
	return nil, ENOSYS
}
