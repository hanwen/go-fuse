// The fuse package provides APIs to implement filesystems in
// userspace.  Typically, each call of the API happens in its own
// goroutine, so take care to make the file system thread-safe.

package fuse

import (
	"github.com/hanwen/go-fuse/raw"
)

// Types for users to implement.

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

	// The name will show up on the output of the mount. Keep this string
	// small.
	Name string

	// If set, wrap the file system in a single-threaded locking wrapper.
	SingleThreaded bool
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
	ReadDirPlus(out *DirEntryList, context *Context, input *raw.ReadIn) Status
	ReleaseDir(context *Context, input *raw.ReleaseIn)
	FsyncDir(context *Context, input *raw.FsyncIn) (code Status)

	//
	StatFs(out *raw.StatfsOut, context *Context) (code Status)

	// This is called on processing the first request. The
	// filesystem implementation can use the server argument to
	// talk back to the kernel (through notify methods).
	Init(*Server)
}
