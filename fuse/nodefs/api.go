// The nodefs package offers a high level API that resembles the
// kernel's idea of what an FS looks like.  File systems can have
// multiple hard-links to one file, for example. It is also suited if
// the data to represent fits in memory: you can construct the
// complete file system tree at mount time
package nodefs

import (
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

// This is a legacy type.
type FileSystem interface {
	// Root should return the inode for root of this file system.
	Root() Node

	// Used for debug outputs
	String() string

	// If called, provide debug output through the log package.
	SetDebug(debug bool)
}

// The Node interface implements the user-defined file system
// functionality
type Node interface {
	// Inode and SetInode are basic getter/setters.  They are
	// called by the FileSystemConnector. You get them for free by
	// embedding the result of NewDefaultNode() in your node
	// struct.
	Inode() *Inode
	SetInode(node *Inode)

	// OnMount is called on the root node just after a mount is
	// executed, either when the actual root is mounted, or when a
	// filesystem is mounted in-process. The passed-in
	// FileSystemConnector gives access to Notify methods and
	// Debug settings.
	OnMount(conn *FileSystemConnector)

	// OnUnmount is executed just before a submount is removed,
	// and when the process receives a forget for the FUSE root
	// node.
	OnUnmount()

	// Lookup finds a child node to this node; it is only called
	// for directory Nodes.
	Lookup(out *fuse.Attr, name string, context *fuse.Context) (*Inode, fuse.Status)

	// Deletable() should return true if this inode may be
	// discarded from the children list. This will be called from
	// within the treeLock critical section, so you cannot look at
	// other inodes.
	Deletable() bool

	// OnForget is called when the reference to this inode is
	// dropped from the tree.
	OnForget()

	// Misc.
	Access(mode uint32, context *fuse.Context) (code fuse.Status)
	Readlink(c *fuse.Context) ([]byte, fuse.Status)

	// Namespace operations; these are only called on directory Nodes.

	// Mknod should create the node, add it to the receiver's
	// inode, and return it
	Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (newNode *Inode, code fuse.Status)

	// Mkdir should create the directory Inode, add it to the
	// receiver's Inode, and return it
	Mkdir(name string, mode uint32, context *fuse.Context) (newNode *Inode, code fuse.Status)
	Unlink(name string, context *fuse.Context) (code fuse.Status)
	Rmdir(name string, context *fuse.Context) (code fuse.Status)

	// Symlink should create a child inode to the receiver, and
	// return it.
	Symlink(name string, content string, context *fuse.Context) (*Inode, fuse.Status)
	Rename(oldName string, newParent Node, newName string, context *fuse.Context) (code fuse.Status)

	// Link should return the Inode of the resulting link. In
	// a POSIX conformant file system, this should add 'existing'
	// to the receiver, and return the Inode corresponding to
	// 'existing'.
	Link(name string, existing Node, context *fuse.Context) (newNode *Inode, code fuse.Status)

	// Create should return an open file, and the Inode for that file.
	Create(name string, flags uint32, mode uint32, context *fuse.Context) (file File, child *Inode, code fuse.Status)
	Open(flags uint32, context *fuse.Context) (file File, code fuse.Status)
	OpenDir(context *fuse.Context) ([]fuse.DirEntry, fuse.Status)

	// XAttrs
	GetXAttr(attribute string, context *fuse.Context) (data []byte, code fuse.Status)
	RemoveXAttr(attr string, context *fuse.Context) fuse.Status
	SetXAttr(attr string, data []byte, flags int, context *fuse.Context) fuse.Status
	ListXAttr(context *fuse.Context) (attrs []string, code fuse.Status)

	// Attributes
	GetAttr(out *fuse.Attr, file File, context *fuse.Context) (code fuse.Status)
	Chmod(file File, perms uint32, context *fuse.Context) (code fuse.Status)
	Chown(file File, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status)
	Truncate(file File, size uint64, context *fuse.Context) (code fuse.Status)
	Utimens(file File, atime *time.Time, mtime *time.Time, context *fuse.Context) (code fuse.Status)
	Fallocate(file File, off uint64, size uint64, mode uint32, context *fuse.Context) (code fuse.Status)

	StatFs() *fuse.StatfsOut
}

// A File object should be returned from FileSystem.Open and
// FileSystem.Create.  Include the NewDefaultFile return value into
// the struct to inherit a default null implementation.
type File interface {
	// Called upon registering the filehandle in the inode.
	SetInode(*Inode)

	// The String method is for debug printing.
	String() string

	// Wrappers around other File implementations, should return
	// the inner file here.
	InnerFile() File

	Read(dest []byte, off int64) (fuse.ReadResult, fuse.Status)
	Write(data []byte, off int64) (written uint32, code fuse.Status)
	Flush() fuse.Status
	Release()
	Fsync(flags int) (code fuse.Status)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	Truncate(size uint64) fuse.Status
	GetAttr(out *fuse.Attr) fuse.Status
	Chown(uid uint32, gid uint32) fuse.Status
	Chmod(perms uint32) fuse.Status
	Utimens(atime *time.Time, mtime *time.Time) fuse.Status
	Allocate(off uint64, size uint64, mode uint32) (code fuse.Status)
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

// Options contains time out options for a node FileSystem.  The
// default copied from libfuse and set in NewMountOptions() is
// (1s,1s,0s).
type Options struct {
	EntryTimeout    time.Duration
	AttrTimeout     time.Duration
	NegativeTimeout time.Duration

	// If set, replace all uids with given UID.
	// NewFileSystemOptions() will set this to the daemon's
	// uid/gid.
	*fuse.Owner

	// If set, use a more portable, but slower inode number
	// generation scheme.  This will make inode numbers (exported
	// back to callers) stay within int32, which is necessary for
	// making stat() succeed in 32-bit programs.
	PortableInodes bool
}
