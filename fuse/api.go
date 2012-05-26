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
	OnUnmount()
	OnMount(conn *FileSystemConnector)
	Root() FsNode

	// Used for debug outputs
	String() string
}

type FsNode interface {
	// The following are called by the FileSystemConnector
	Inode() *Inode
	SetInode(node *Inode)

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

	// Namespace operations
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
	Utimens(file File, atime int64, mtime int64, context *Context) (code Status)

	StatFs() *StatfsOut
}

// A filesystem API that uses paths rather than inodes.  A minimal
// file system should have at least a functional GetAttr method.
// Typically, each call happens in its own goroutine, so take care to
// make the file system thread-safe.
//
// Include DefaultFileSystem to provide a default null implementation of
// required methods.
type FileSystem interface {
	// Used for pretty printing.
	String() string

	// Attributes.  This function is the main entry point, through
	// which FUSE discovers which files and directories exist.
	//
	// If the filesystem wants to implement hard-links, it should
	// return consistent non-zero FileInfo.Ino data.  Using
	// hardlinks incurs a performance hit.
	GetAttr(name string, context *Context) (*Attr, Status)

	// These should update the file's ctime too.
	Chmod(name string, mode uint32, context *Context) (code Status)
	Chown(name string, uid uint32, gid uint32, context *Context) (code Status)
	Utimens(name string, AtimeNs int64, MtimeNs int64, context *Context) (code Status)

	Truncate(name string, size uint64, context *Context) (code Status)

	Access(name string, mode uint32, context *Context) (code Status)

	// Tree structure
	Link(oldName string, newName string, context *Context) (code Status)
	Mkdir(name string, mode uint32, context *Context) Status
	Mknod(name string, mode uint32, dev uint32, context *Context) Status
	Rename(oldName string, newName string, context *Context) (code Status)
	Rmdir(name string, context *Context) (code Status)
	Unlink(name string, context *Context) (code Status)

	// Extended attributes.
	GetXAttr(name string, attribute string, context *Context) (data []byte, code Status)
	ListXAttr(name string, context *Context) (attributes []string, code Status)
	RemoveXAttr(name string, attr string, context *Context) Status
	SetXAttr(name string, attr string, data []byte, flags int, context *Context) Status

	// Called after mount.
	OnMount(nodeFs *PathNodeFs)
	OnUnmount()

	// File handling.  If opening for writing, the file's mtime
	// should be updated too.
	Open(name string, flags uint32, context *Context) (file File, code Status)
	Create(name string, flags uint32, mode uint32, context *Context) (file File, code Status)

	// Directory handling
	OpenDir(name string, context *Context) (stream []DirEntry, code Status)

	// Symlinks.
	Symlink(value string, linkName string, context *Context) (code Status)
	Readlink(name string, context *Context) (string, Status)

	StatFs(name string) *StatfsOut
}

type PathNodeFsOptions struct {
	// If ClientInodes is set, use Inode returned from GetAttr to
	// find hard-linked files.
	ClientInodes bool
}

// A File object should be returned from FileSystem.Open and
// FileSystem.Create.  Include DefaultFile into the struct to inherit
// a default null implementation.  
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

	Read(dest []byte, off int64) ReadResult
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
	Utimens(atimeNs int64, mtimeNs int64) Status
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
}

// DefaultFileSystem implements a FileSystem that returns ENOSYS for every operation.
type DefaultFileSystem struct{}

// DefaultFile returns ENOSYS for every operation.
type DefaultFile struct{}

// RawFileSystem is an interface close to the FUSE wire protocol.
//
// Unless you really know what you are doing, you should not implement
// this, but rather the FileSystem interface; the details of getting
// interactions with open files, renames, and threading right etc. are
// somewhat tricky and not very interesting.
//
// Include DefaultRawFileSystem to inherit a null implementation.
type RawFileSystem interface {
	Lookup(out *raw.EntryOut, header *raw.InHeader, name string) (status Status)
	Forget(nodeid, nlookup uint64)

	// Attributes.
	GetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.GetAttrIn) (code Status)
	SetAttr(out *raw.AttrOut, header *raw.InHeader, input *raw.SetAttrIn) (code Status)

	// Modifying structure.
	Mknod(out *raw.EntryOut, header *raw.InHeader, input *raw.MknodIn, name string) (code Status)
	Mkdir(out *raw.EntryOut, header *raw.InHeader, input *raw.MkdirIn, name string) (code Status)
	Unlink(header *raw.InHeader, name string) (code Status)
	Rmdir(header *raw.InHeader, name string) (code Status)
	Rename(header *raw.InHeader, input *raw.RenameIn, oldName string, newName string) (code Status)
	Link(out *raw.EntryOut, header *raw.InHeader, input *raw.LinkIn, filename string) (code Status)

	Symlink(out *raw.EntryOut, header *raw.InHeader, pointedTo string, linkName string) (code Status)
	Readlink(header *raw.InHeader) (out []byte, code Status)
	Access(header *raw.InHeader, input *raw.AccessIn) (code Status)

	// Extended attributes.
	GetXAttrSize(header *raw.InHeader, attr string) (sz int, code Status)
	GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status)
	ListXAttr(header *raw.InHeader) (attributes []byte, code Status)
	SetXAttr(header *raw.InHeader, input *raw.SetXAttrIn, attr string, data []byte) Status
	RemoveXAttr(header *raw.InHeader, attr string) (code Status)

	// File handling.
	Create(out *raw.CreateOut, header *raw.InHeader, input *raw.CreateIn, name string) (code Status)
	Open(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status)
	Read(*raw.InHeader, *raw.ReadIn, []byte) ReadResult

	Release(header *raw.InHeader, input *raw.ReleaseIn)
	Write(*raw.InHeader, *raw.WriteIn, []byte) (written uint32, code Status)
	Flush(header *raw.InHeader, input *raw.FlushIn) Status
	Fsync(*raw.InHeader, *raw.FsyncIn) (code Status)

	// Directory handling
	OpenDir(out *raw.OpenOut, header *raw.InHeader, input *raw.OpenIn) (status Status)
	ReadDir(out *DirEntryList, header *raw.InHeader, input *raw.ReadIn) Status
	ReleaseDir(header *raw.InHeader, input *raw.ReleaseIn)
	FsyncDir(header *raw.InHeader, input *raw.FsyncIn) (code Status)

	//
	StatFs(out *StatfsOut, eader *raw.InHeader) (code Status)

	// Provide callbacks for pushing notifications to the kernel.
	Init(params *RawFsInit)
}

// DefaultRawFileSystem returns ENOSYS for every operation.
type DefaultRawFileSystem struct{}

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
	InodeNotify func(*raw.NotifyInvalInodeOut) Status
	EntryNotify func(parent uint64, name string) Status
}
