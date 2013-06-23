// The fuse package provides APIs to implement filesystems in
// userspace.  Typically, each call of the API happens in its own
// goroutine, so take care to make the file system thread-safe.
package fuse

import (
	"time"

	"github.com/hanwen/go-fuse/raw"
)

// Types for users to implement.

// NodeFileSystem is a high level API that resembles the kernel's idea
// of what an FS looks like.  NodeFileSystems can have multiple
// hard-links to one file, for example. It is also suited if the data
// to represent fits in memory: you can construct FsNode at mount
// time, and the filesystem will be ready.
type NodeFileSystem interface {
	// OnUnmount is executed just before a submount is removed,
	// and when the process receives a forget for the FUSE root
	// node.
	OnUnmount()

	// OnMount is called just after a mount is executed, either
	// when the root is mounted, or when other filesystem are
	// mounted in-process. The passed-in FileSystemConnector gives
	// access to Notify methods and Debug settings.
	OnMount(conn *FileSystemConnector)

	// Root should return the inode for root of this file system.
	Root() FsNode

	// Used for debug outputs
	String() string

	// If called, provide debug output through the log package.
	SetDebug(debug bool) 
}

// The FsNode implements the basic functionality of inodes; this is
// where the majority of the FS code for a typical filesystem will be.
type FsNode interface {
	// Inode and SetInode are basic getter/setters.  They are
	// called by the FileSystemConnector. You get them for free by
	// embedding DefaultFsNode.
	Inode() *Inode
	SetInode(node *Inode)

	// Lookup finds a child node to this node; it is only called
	// for directory FsNodes.
	Lookup(out *Attr, name string, context *Context) (node FsNode, code Status)

	// Deletable() should return true if this inode may be
	// discarded from the children list. This will be called from
	// within the treeLock critical section, so you cannot look at
	// other inodes.
	Deletable() bool

	// OnForget is called when the reference to this inode is
	// dropped from the tree.
	OnForget()

	// Misc.
	Access(mode uint32, context *Context) (code Status)
	Readlink(c *Context) ([]byte, Status)

	// Namespace operations; these are only called on directory FsNodes.
	Mknod(name string, mode uint32, dev uint32, context *Context) (newNode FsNode, code Status)
	Mkdir(name string, mode uint32, context *Context) (newNode FsNode, code Status)
	Unlink(name string, context *Context) (code Status)
	Rmdir(name string, context *Context) (code Status)
	Symlink(name string, content string, context *Context) (newNode FsNode, code Status)
	Rename(oldName string, newParent FsNode, newName string, context *Context) (code Status)
	Link(name string, existing FsNode, context *Context) (newNode FsNode, code Status)

	// Files
	Create(name string, flags uint32, mode uint32, context *Context) (file File, newNode FsNode, code Status)
	Open(flags uint32, context *Context) (file File, code Status)
	OpenDir(context *Context) ([]DirEntry, Status)

	// XAttrs
	GetXAttr(attribute string, context *Context) (data []byte, code Status)
	RemoveXAttr(attr string, context *Context) Status
	SetXAttr(attr string, data []byte, flags int, context *Context) Status
	ListXAttr(context *Context) (attrs []string, code Status)

	// Attributes
	GetAttr(out *Attr, file File, context *Context) (code Status)
	Chmod(file File, perms uint32, context *Context) (code Status)
	Chown(file File, uid uint32, gid uint32, context *Context) (code Status)
	Truncate(file File, size uint64, context *Context) (code Status)
	Utimens(file File, atime *time.Time, mtime *time.Time, context *Context) (code Status)
	Fallocate(file File, off uint64, size uint64, mode uint32, context *Context) (code Status)

	StatFs() *StatfsOut
}

// A File object should be returned from FileSystem.Open and
// FileSystem.Create.  Include the NewDefaultFile return value into
// the struct to inherit a default null implementation.
//
// TODO - should File be thread safe?
// TODO - should we pass a *Context argument?
type File interface {
	// Called upon registering the filehandle in the inode.
	SetInode(*Inode)

	// The String method is for debug printing.
	String() string

	// Wrappers around other File implementations, should return
	// the inner file here.
	InnerFile() File

	Read(dest []byte, off int64) (ReadResult, Status)
	Write(data []byte, off int64) (written uint32, code Status)
	Flush() Status
	Release()
	Fsync(flags int) (code Status)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	Truncate(size uint64) Status
	GetAttr(out *Attr) Status
	Chown(uid uint32, gid uint32) Status
	Chmod(perms uint32) Status
	Utimens(atime *time.Time, mtime *time.Time) Status
	Allocate(off uint64, size uint64, mode uint32) (code Status)
}

// The result of Read is an array of bytes, but for performance
// reasons, we can also return data as a file-descriptor/offset/size
// tuple.  If the backing store for a file is another filesystem, this
// reduces the amount of copying between the kernel and the FUSE
// server.  The ReadResult interface captures both cases.
type ReadResult interface {
	// Returns the raw bytes for the read, possibly using the
	// passed buffer. The buffer should be larger than the return
	// value from Size.
	Bytes(buf []byte) ([]byte, Status)

	// Size returns how many bytes this return value takes at most.
	Size() int

	// Done() is called after sending the data to the kernel.
	Done()
}

// Wrap a File return in this to set FUSE flags.  Also used internally
// to store open file data.
type WithFlags struct {
	File

	// For debugging.
	Description string

	// Put FOPEN_* flags here.
	FuseFlags uint32

	// O_RDWR, O_TRUNCATE, etc.
	OpenFlags uint32
}

// MountOptions contains time out options for a (Node)FileSystem.  The
// default copied from libfuse and set in NewMountOptions() is
// (1s,1s,0s).
type FileSystemOptions struct {
	EntryTimeout    time.Duration
	AttrTimeout     time.Duration
	NegativeTimeout time.Duration

	// If set, replace all uids with given UID.
	// NewFileSystemOptions() will set this to the daemon's
	// uid/gid.
	*Owner

	// If set, use a more portable, but slower inode number
	// generation scheme.  This will make inode numbers (exported
	// back to callers) stay within int32, which is necessary for
	// making stat() succeed in 32-bit programs.
	PortableInodes bool
}

type MountOptions struct {
	AllowOther bool

	// Options are passed as -o string to fusermount.
	Options []string

	// Default is _DEFAULT_BACKGROUND_TASKS, 12.  This numbers
	// controls the allowed number of requests that relate to
	// async I/O.  Concurrency for synchronous I/O is not limited.
	MaxBackground int

	// Write size to use.  If 0, use default. This number is
	// capped at the kernel maximum.
	MaxWrite int

	// If IgnoreSecurityLabels is set, all security related xattr
	// requests will return NO_DATA without passing through the
	// user defined filesystem.  You should only set this if you
	// file system implements extended attributes, and you are not
	// interested in security labels.
	IgnoreSecurityLabels bool // ignoring labels should be provided as a fusermount mount option.

	// If given, use this buffer pool instead of the global one.
	Buffers BufferPool

	// If RememberInodes is set, we will never forget inodes.
	// This may be useful for NFS.
	RememberInodes bool

	// The Name will show up on the output of the mount. Keep this string
	// small.
	Name string
}

// RawFileSystem is an interface close to the FUSE wire protocol.
//
// Unless you really know what you are doing, you should not implement
// this, but rather the FileSystem interface; the details of getting
// interactions with open files, renames, and threading right etc. are
// somewhat tricky and not very interesting.
//
// A null implementation is provided by NewDefaultRawFileSystem.
type RawFileSystem interface {
	String() string

	// If called, provide debug output through the log package.
	SetDebug(debug bool) 

	Lookup(out *raw.EntryOut, context *Context, name string) (status Status)
	Forget(nodeid, nlookup uint64)

	// Attributes.
	GetAttr(out *raw.AttrOut, context *Context, input *raw.GetAttrIn) (code Status)
	SetAttr(out *raw.AttrOut, context *Context, input *raw.SetAttrIn) (code Status)

	// Modifying structure.
	Mknod(out *raw.EntryOut, context *Context, input *raw.MknodIn, name string) (code Status)
	Mkdir(out *raw.EntryOut, context *Context, input *raw.MkdirIn, name string) (code Status)
	Unlink(context *Context, name string) (code Status)
	Rmdir(context *Context, name string) (code Status)
	Rename(context *Context, input *raw.RenameIn, oldName string, newName string) (code Status)
	Link(out *raw.EntryOut, context *Context, input *raw.LinkIn, filename string) (code Status)

	Symlink(out *raw.EntryOut, context *Context, pointedTo string, linkName string) (code Status)
	Readlink(context *Context) (out []byte, code Status)
	Access(context *Context, input *raw.AccessIn) (code Status)

	// Extended attributes.
	GetXAttrSize(context *Context, attr string) (sz int, code Status)
	GetXAttrData(context *Context, attr string) (data []byte, code Status)
	ListXAttr(context *Context) (attributes []byte, code Status)
	SetXAttr(context *Context, input *raw.SetXAttrIn, attr string, data []byte) Status
	RemoveXAttr(context *Context, attr string) (code Status)

	// File handling.
	Create(out *raw.CreateOut, context *Context, input *raw.CreateIn, name string) (code Status)
	Open(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status)
	Read(*Context, *raw.ReadIn, []byte) (ReadResult, Status)

	Release(context *Context, input *raw.ReleaseIn)
	Write(*Context, *raw.WriteIn, []byte) (written uint32, code Status)
	Flush(context *Context, input *raw.FlushIn) Status
	Fsync(*Context, *raw.FsyncIn) (code Status)
	Fallocate(Context *Context, in *raw.FallocateIn) (code Status)

	// Directory handling
	OpenDir(out *raw.OpenOut, context *Context, input *raw.OpenIn) (status Status)
	ReadDir(out *DirEntryList, context *Context, input *raw.ReadIn) Status
	ReleaseDir(context *Context, input *raw.ReleaseIn)
	FsyncDir(context *Context, input *raw.FsyncIn) (code Status)

	//
	StatFs(out *StatfsOut, context *Context) (code Status)

	// Provide callbacks for pushing notifications to the kernel.
	Init(params *RawFsInit)
}

// Talk back to FUSE.
//
// InodeNotify invalidates the information associated with the inode
// (ie. data cache, attributes, etc.)
//
// EntryNotify should be used if the existence status of an entry changes,
// (ie. to notify of creation or deletion of the file).
//
// Somewhat confusingly, InodeNotify for a file that stopped to exist
// will give the correct result for Lstat (ENOENT), but the kernel
// will still issue file Open() on the inode.
type RawFsInit struct {
	InodeNotify  func(*raw.NotifyInvalInodeOut) Status
	EntryNotify  func(parent uint64, name string) Status
	DeleteNotify func(parent uint64, child uint64, name string) Status
}
