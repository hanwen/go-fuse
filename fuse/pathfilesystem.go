package fuse

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
)

type mountData struct {
	// If non-nil the file system mounted here.
	fs PathFilesystem

	// Protects the variables below.
	mutex sync.RWMutex

	// If yes, we are looking to unmount the mounted fs.
	unmountPending bool
}


func newMount(fs PathFilesystem) *mountData {
	return &mountData{fs: fs}
}

// Tests should set to true.
var paranoia = false

// TODO should rename to dentry?
type inode struct {
	Parent      *inode
	Children    map[string]*inode
	NodeId      uint64
	Name        string
	LookupCount int
	OpenCount   int
	Type        uint32 // dirent type, used to check if mounts are valid.

	mount *mountData
}

// Should be called with lock held.
func (me *inode) totalOpenCount() int {
	o := me.OpenCount
	for _, v := range me.Children {
		o += v.totalOpenCount()
	}
	return o
}


const initDirSize = 20

func (me *inode) verify() {
	if !(me.NodeId == FUSE_ROOT_ID || me.LookupCount > 0 || len(me.Children) > 0) {
		panic(fmt.Sprintf("node should be dead: %v", me))
	}
	for n, ch := range me.Children {
		if ch == nil {
			panic("Found nil child.")
		}
		if ch.Name != n {
			panic(fmt.Sprintf("parent/child name corrupted %v %v",
				ch.Name, n))
		}
		if ch.Parent != me {
			panic(fmt.Sprintf("parent/child relation corrupted %v %v %v",
				ch.Parent, me, ch))
		}
	}
}

func (me *inode) GetPath() (path string, mount *mountData) {
	rev_components := make([]string, 0, 10)
	inode := me

	for ; inode != nil && inode.mount == nil; inode = inode.Parent {
		rev_components = append(rev_components, inode.Name)
	}
	if inode == nil {
		panic("did not find parent with mount")
	}
	mount = inode.mount

	mount.mutex.RLock()
	defer mount.mutex.RUnlock()
	if mount.unmountPending {
		return "", nil
	}
	components := make([]string, len(rev_components))
	for i, v := range rev_components {
		components[len(rev_components)-i-1] = v
	}
	fullPath := strings.Join(components, "/")
	return fullPath, mount
}

// Must be called with lock held.
func (me *inode) setParent(newParent *inode) {
	if me.Parent == newParent {
		return
	}

	if me.Parent != nil {
		me.Parent.Children[me.Name] = nil, false
		me.Parent = nil
	}
	if newParent != nil {
		me.Parent = newParent
		ch := me.Parent.Children[me.Name]
		if ch != nil {
			panic(fmt.Sprintf("Already have an inode with same name: %v.", me.Name))
		}

		me.Parent.Children[me.Name] = me
	}
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
	DefaultRawFuseFileSystem

	options PathFileSystemConnectorOptions
	Debug   bool

	////////////////

	// Protects the inode hashmap, its contents and the nextFreeInode counter.
	lock sync.RWMutex

	// Invariants: see the verify() method.
	inodeMap      map[uint64]*inode
	nextFreeInode uint64

	// Open files/directories.
	fileLock       sync.RWMutex
	openFiles      map[uint64]interface{}
	nextFreeHandle uint64
}

func (me *PathFileSystemConnector) unregisterFile(node *inode, handle uint64) interface{} {
	me.fileLock.Lock()
	defer me.fileLock.Unlock()
	f, ok := me.openFiles[handle]
	if !ok {
		panic("invalid handle")
	}
	me.openFiles[handle] = nil, false
	node.OpenCount--
	return f
}

func (me *PathFileSystemConnector) registerFile(node *inode, f interface{}) uint64 {
	me.fileLock.Lock()
	defer me.fileLock.Unlock()

	h := me.nextFreeHandle
	me.nextFreeHandle++
	_, ok := me.openFiles[h]
	if ok {
		panic("handle counter wrapped")
	}

	node.OpenCount++
	me.openFiles[h] = f
	return h
}

func (me *PathFileSystemConnector) getDir(h uint64) RawFuseDir {
	me.fileLock.RLock()
	defer me.fileLock.RUnlock()
	return me.openFiles[h].(RawFuseDir)
}

func (me *PathFileSystemConnector) getFile(h uint64) FuseFile {
	me.fileLock.RLock()
	defer me.fileLock.RUnlock()
	return me.openFiles[h].(FuseFile)
}

func (me *PathFileSystemConnector) verify() {
	if !paranoia {
		return
	}
	for k, v := range me.inodeMap {
		if v.NodeId != k {
			panic(fmt.Sprintf("nodeid mismatch %v %v", v, k))
		}
	}
	me.inodeMap[FUSE_ROOT_ID].verify()
}

func (me *PathFileSystemConnector) newInode() *inode {
	data := new(inode)
	data.NodeId = me.nextFreeInode
	me.nextFreeInode++

	me.inodeMap[data.NodeId] = data

	return data
}

func (me *PathFileSystemConnector) lookupUpdate(parent *inode, name string, isDir bool) *inode {
	defer me.verify()

	me.lock.Lock()
	defer me.lock.Unlock()

	data, ok := parent.Children[name]
	if !ok {
		data = me.newInode()
		data.Name = name
		data.setParent(parent)
		if isDir {
			data.Children = make(map[string]*inode, initDirSize)
		}
	}

	return data
}

func (me *PathFileSystemConnector) getInodeData(nodeid uint64) *inode {
	me.lock.RLock()
	defer me.lock.RUnlock()

	val := me.inodeMap[nodeid]
	if val == nil {
		panic(fmt.Sprintf("inode %v unknown", nodeid))
	}
	return val
}

func (me *PathFileSystemConnector) forgetUpdate(nodeId uint64, forgetCount int) {
	defer me.verify()
	me.lock.Lock()
	defer me.lock.Unlock()

	data, ok := me.inodeMap[nodeId]
	if ok {
		data.LookupCount -= forgetCount

		if data.mount != nil {
			data.mount.mutex.RLock()
			defer data.mount.mutex.RUnlock()
		}

		// TODO - this should probably not happen at all.
		if data.LookupCount <= 0 && len(data.Children) == 0 && (data.mount == nil || data.mount.unmountPending) {
			data.setParent(nil)
			me.inodeMap[nodeId] = nil, false
		}
	}
}

func (me *PathFileSystemConnector) renameUpdate(oldParent *inode, oldName string, newParent *inode, newName string) {
	defer me.verify()
	me.lock.Lock()
	defer me.lock.Unlock()

	node := oldParent.Children[oldName]
	if node == nil {
		panic("Source of rename does not exist")
	}

	node.setParent(nil)
	node.Name = newName
	node.setParent(newParent)
}

func (me *PathFileSystemConnector) unlinkUpdate(parent *inode, name string) {
	defer me.verify()
	me.lock.Lock()
	defer me.lock.Unlock()

	node := parent.Children[name]
	node.setParent(nil)
}

// Walk the file system starting from the root.
func (me *PathFileSystemConnector) findInode(fullPath string) *inode {
	fullPath = strings.TrimLeft(filepath.Clean(fullPath), "/")
	comps := strings.Split(fullPath, "/", -1)

	me.lock.RLock()
	defer me.lock.RUnlock()

	node := me.inodeMap[FUSE_ROOT_ID]
	for i, component := range comps {
		if len(component) == 0 {
			continue
		}

		node = node.Children[component]
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
	out.inodeMap = make(map[uint64]*inode)
	out.openFiles = make(map[uint64]interface{})

	out.nextFreeInode = FUSE_ROOT_ID
	rootData := out.newInode()
	rootData.NodeId = FUSE_ROOT_ID
	rootData.Type = ModeToType(S_IFDIR)
	rootData.Children = make(map[string]*inode, initDirSize)

	out.options.NegativeTimeout = 0.0
	out.options.AttrTimeout = 1.0
	out.options.EntryTimeout = 1.0

	if code := out.Mount("/", fs); code != OK {
		panic("root mount failed.")
	}

	out.verify()

	return out
}

func (me *PathFileSystemConnector) SetOptions(opts PathFileSystemConnectorOptions) {
	me.options = opts
}

func (me *PathFileSystemConnector) Mount(mountPoint string, fs PathFilesystem) Status {
	var node *inode

	if mountPoint != "/" {
		dirParent, base := filepath.Split(mountPoint)
		dirParentNode := me.findInode(dirParent)

		// Make sure we know the mount point.
		_, _ = me.internalLookup(dirParentNode, base, 0)
	}

	node = me.findInode(mountPoint)
	if node.Type&ModeToType(S_IFDIR) == 0 {
		return EINVAL
	}

	me.lock.Lock()
	hasChildren := len(node.Children) > 0
	// don't use defer, as we dont want to hold the lock during
	// fs.Mount().
	me.lock.Unlock()

	if hasChildren {
		return EBUSY
	}

	code := fs.Mount(me)
	if code != OK {
		if me.Debug {
			log.Println("Mount error: ", mountPoint, code)
		}
		return code
	}

	if me.Debug {
		log.Println("Mount: ", fs, "on", mountPoint, node)
	}

	node.mount = newMount(fs)

	me.fileLock.Lock()
	defer me.fileLock.Unlock()
	if node.Parent != nil {
		node.Parent.OpenCount++
	}

	return OK
}

func (me *PathFileSystemConnector) Unmount(path string) Status {
	node := me.findInode(path)
	if node == nil {
		panic(path)
	}

	mount := node.mount
	if mount == nil {
		panic(path)
	}

	// Need to lock to look at node.Children
	me.lock.RLock()
	defer me.lock.RUnlock()

	me.fileLock.Lock()
	defer me.fileLock.Unlock()

	if node.totalOpenCount() > 0 {
		log.Println("Umount - busy: ", mount)
		return EBUSY
	}

	if me.Debug {
		log.Println("Unmount: ", mount)
	}

	mount.mutex.Lock()
	defer mount.mutex.Unlock()
	if len(node.Children) > 0 {
		mount.fs.Unmount()
		mount.unmountPending = true
	} else {
		node.mount = nil
	}

	if node.Parent != nil {
		node.Parent.OpenCount--
	}
	return OK
}

func (me *PathFileSystemConnector) GetPath(nodeid uint64) (path string, mount *mountData, node *inode) {
	n := me.getInodeData(nodeid)
	p, m := n.GetPath()
	return p, m, n
}

func (me *PathFileSystemConnector) Init(h *InHeader, input *InitIn) (*InitOut, Status) {
	// TODO ?
	return new(InitOut), OK
}

func (me *PathFileSystemConnector) Destroy(h *InHeader, input *InitIn) {
	// TODO - umount all.
}

func (me *PathFileSystemConnector) Lookup(header *InHeader, name string) (out *EntryOut, status Status) {
	parent := me.getInodeData(header.NodeId)
	return me.internalLookup(parent, name, 1)
}

func (me *PathFileSystemConnector) internalLookup(parent *inode, name string, lookupCount int) (out *EntryOut, status Status) {
	out, status, _ = me.internalLookupWithNode(parent, name, lookupCount)
	return out, status
}

func (me *PathFileSystemConnector) internalLookupWithNode(parent *inode, name string, lookupCount int) (out *EntryOut, status Status, node *inode) {
	// TODO - fuse.c has special case code for name == "." and
	// "..", those lookups happen if FUSE_EXPORT_SUPPORT is set in
	// Init.
	fullPath, mount := parent.GetPath()
	if mount == nil {
		return NegativeEntry(me.options.NegativeTimeout), OK, nil
	}
	fullPath = filepath.Join(fullPath, name)

	attr, err := mount.fs.GetAttr(fullPath)

	if err == ENOENT && me.options.NegativeTimeout > 0.0 {
		return NegativeEntry(me.options.NegativeTimeout), OK, nil
	}

	if err != OK {
		return nil, err, nil
	}

	data := me.lookupUpdate(parent, name, attr.Mode&S_IFDIR != 0)
	data.LookupCount += lookupCount
	data.Type = ModeToType(attr.Mode)

	out = new(EntryOut)
	out.NodeId = data.NodeId
	out.Generation = 1 // where to get the generation?

	SplitNs(me.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	out.Attr = *attr
	out.Attr.Ino = data.NodeId
	return out, OK, data
}

func (me *PathFileSystemConnector) Forget(h *InHeader, input *ForgetIn) {
	me.forgetUpdate(h.NodeId, int(input.Nlookup))
}

func (me *PathFileSystemConnector) GetAttr(header *InHeader, input *GetAttrIn) (out *AttrOut, code Status) {
	// TODO - do something intelligent with input.Fh.
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	attr, err := mount.fs.GetAttr(fullPath)
	if err != OK {
		return nil, err
	}

	out = new(AttrOut)
	out.Attr = *attr
	out.Attr.Ino = header.NodeId
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)

	return out, OK
}

func (me *PathFileSystemConnector) OpenDir(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, ENOENT
	}
	// TODO - how to handle return flags, the FUSE open flags?
	stream, err := mount.fs.OpenDir(fullPath)
	if err != OK {
		return 0, 0, err
	}

	de := new(FuseDir)
	de.stream = stream

	h := me.registerFile(node, de)

	return 0, h, OK
}

func (me *PathFileSystemConnector) ReadDir(header *InHeader, input *ReadIn) (*DirEntryList, Status) {
	d := me.getDir(input.Fh)
	de, code := d.ReadDir(input)
	if code != OK {
		return nil, code
	}
	return de, OK
}

func (me *PathFileSystemConnector) Open(header *InHeader, input *OpenIn) (flags uint32, handle uint64, status Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, ENOENT
	}

	// TODO - how to handle return flags, the FUSE open flags?
	f, err := mount.fs.Open(fullPath, input.Flags)
	if err != OK {
		return 0, 0, err
	}
	h := me.registerFile(node, f)

	return 0, h, OK
}

func (me *PathFileSystemConnector) SetAttr(header *InHeader, input *SetAttrIn) (out *AttrOut, code Status) {
	var err Status = OK

	// TODO - support Fh.   (FSetAttr/FGetAttr/FTruncate.)
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	if input.Valid&FATTR_MODE != 0 {
		err = mount.fs.Chmod(fullPath, input.Mode)
	}
	if err != OK && (input.Valid&FATTR_UID != 0 || input.Valid&FATTR_GID != 0) {
		// TODO - can we get just FATTR_GID but not FATTR_UID ?
		err = mount.fs.Chown(fullPath, uint32(input.Uid), uint32(input.Gid))
	}
	if input.Valid&FATTR_SIZE != 0 {
		mount.fs.Truncate(fullPath, input.Size)
	}
	if err != OK && (input.Valid&FATTR_ATIME != 0 || input.Valid&FATTR_MTIME != 0) {
		err = mount.fs.Utimens(fullPath,
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
	return me.GetAttr(header, new(GetAttrIn))
}

func (me *PathFileSystemConnector) Readlink(header *InHeader) (out []byte, code Status) {
	fullPath, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	val, err := mount.fs.Readlink(fullPath)
	return bytes.NewBufferString(val).Bytes(), err
}

func (me *PathFileSystemConnector) Mknod(header *InHeader, input *MknodIn, name string) (out *EntryOut, code Status) {
	fullPath, mount, node := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	fullPath = filepath.Join(fullPath, name)
	err := mount.fs.Mknod(fullPath, input.Mode, uint32(input.Rdev))
	if err != OK {
		return nil, err
	}
	return me.internalLookup(node, name, 1)
}

func (me *PathFileSystemConnector) Mkdir(header *InHeader, input *MkdirIn, name string) (out *EntryOut, code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	code = mount.fs.Mkdir(filepath.Join(fullPath, name), input.Mode)
	if code == OK {
		out, code = me.internalLookup(parent, name, 1)
	}
	return out, code
}

func (me *PathFileSystemConnector) Unlink(header *InHeader, name string) (code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	code = mount.fs.Unlink(filepath.Join(fullPath, name))
	if code == OK {
		// Like fuse.c, we update our internal tables.
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *PathFileSystemConnector) Rmdir(header *InHeader, name string) (code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	code = mount.fs.Rmdir(filepath.Join(fullPath, name))
	if code == OK {
		me.unlinkUpdate(parent, name)
	}
	return code
}

func (me *PathFileSystemConnector) Symlink(header *InHeader, pointedTo string, linkName string) (out *EntryOut, code Status) {
	fullPath, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}
	err := mount.fs.Symlink(pointedTo, filepath.Join(fullPath, linkName))
	if err != OK {
		return nil, err
	}

	out, code = me.internalLookup(parent, linkName, 1)
	return out, code
}

func (me *PathFileSystemConnector) Rename(header *InHeader, input *RenameIn, oldName string, newName string) (code Status) {
	oldPath, oldMount, oldParent := me.GetPath(header.NodeId)
	newPath, mount, newParent := me.GetPath(input.Newdir)
	if mount == nil || oldMount == nil {
		return ENOENT
	}
	if mount != oldMount {
		return EXDEV
	}

	oldPath = filepath.Join(oldPath, oldName)
	newPath = filepath.Join(newPath, newName)
	code = mount.fs.Rename(oldPath, newPath)
	if code == OK {
		// It is conceivable that the kernel module will issue a
		// forget for the old entry, and a lookup request for the new
		// one, but the fuse.c updates its client-side tables on its
		// own, so we do this as well.
		//
		// It should not hurt for us to do it here as well, although
		// it remains unclear how we should update Count.
		me.renameUpdate(oldParent, oldName, newParent, newName)
	}
	return code
}

func (me *PathFileSystemConnector) Link(header *InHeader, input *LinkIn, filename string) (out *EntryOut, code Status) {
	orig, mount, _ := me.GetPath(input.Oldnodeid)
	newName, newMount, newParent := me.GetPath(header.NodeId)

	if mount == nil || newMount == nil {
		return nil, ENOENT
	}
	if mount != newMount {
		return nil, EXDEV
	}
	newName = filepath.Join(newName, filename)
	err := mount.fs.Link(orig, newName)

	if err != OK {
		return nil, err
	}

	return me.internalLookup(newParent, filename, 1)
}

func (me *PathFileSystemConnector) Access(header *InHeader, input *AccessIn) (code Status) {
	p, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}
	return mount.fs.Access(p, input.Mask)
}

func (me *PathFileSystemConnector) Create(header *InHeader, input *CreateIn, name string) (flags uint32, h uint64, out *EntryOut, code Status) {
	directory, mount, parent := me.GetPath(header.NodeId)
	if mount == nil {
		return 0, 0, nil, ENOENT
	}
	fullPath := filepath.Join(directory, name)

	f, err := mount.fs.Create(fullPath, uint32(input.Flags), input.Mode)
	if err != OK {
		return 0, 0, nil, err
	}

	out, code, inode := me.internalLookupWithNode(parent, name, 1)
	return 0, me.registerFile(inode, f), out, code
}

func (me *PathFileSystemConnector) Release(header *InHeader, input *ReleaseIn) {
	_, _, node := me.GetPath(header.NodeId)
	f := me.unregisterFile(node, input.Fh).(FuseFile)
	f.Release()
}

func (me *PathFileSystemConnector) ReleaseDir(header *InHeader, input *ReleaseIn) {
	_, _, node := me.GetPath(header.NodeId)
	d := me.unregisterFile(node, input.Fh).(RawFuseDir)
	d.Release()
}

func (me *PathFileSystemConnector) FsyncDir(header *InHeader, input *FsyncIn) (code Status) {
	// What the heck is FsyncDir supposed to do?
	return OK
}

func (me *PathFileSystemConnector) GetXAttr(header *InHeader, attribute string) (data []byte, code Status) {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	data, code = mount.fs.GetXAttr(path, attribute)
	return data, code
}

func (me *PathFileSystemConnector) RemoveXAttr(header *InHeader, attr string) Status {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}

	return mount.fs.RemoveXAttr(path, attr)
}

func (me *PathFileSystemConnector) SetXAttr(header *InHeader, input *SetXAttrIn, attr string, data []byte) Status {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return ENOENT
	}

	return mount.fs.SetXAttr(path, attr, data, int(input.Flags))
}

func (me *PathFileSystemConnector) ListXAttr(header *InHeader) (data []byte, code Status) {
	path, mount, _ := me.GetPath(header.NodeId)
	if mount == nil {
		return nil, ENOENT
	}

	attrs, code := mount.fs.ListXAttr(path)
	if code != OK {
		return nil, code
	}

	b := bytes.NewBuffer([]byte{})
	for _, v := range attrs {
		b.Write([]byte(v))
		b.WriteByte(0)
	}

	return b.Bytes(), code
}

func (me *PathFileSystemConnector) Write(input *WriteIn, data []byte) (written uint32, code Status) {
	f := me.getFile(input.Fh).(FuseFile)
	return f.Write(input, data)
}

func (me *PathFileSystemConnector) Read(input *ReadIn, bp *BufferPool) ([]byte, Status) {
	f := me.getFile(input.Fh)
	return f.Read(input, bp)
}
