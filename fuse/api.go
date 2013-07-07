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

	Lookup(header *raw.InHeader, name string, out *raw.EntryOut) (status Status)
	Forget(nodeid, nlookup uint64)

	// Attributes.
	GetAttr(input *raw.GetAttrIn, out *raw.AttrOut) (code Status)
	SetAttr(input *raw.SetAttrIn, out *raw.AttrOut) (code Status)

	// Modifying structure.
	Mknod(input *raw.MknodIn, name string, out *raw.EntryOut) (code Status)
	Mkdir(input *raw.MkdirIn, name string, out *raw.EntryOut) (code Status)
	Unlink(header *raw.InHeader, name string) (code Status)
	Rmdir(header *raw.InHeader, name string) (code Status)
	Rename(input *raw.RenameIn, oldName string, newName string) (code Status)
	Link(input *raw.LinkIn, filename string, out *raw.EntryOut) (code Status)

	Symlink(header *raw.InHeader, pointedTo string, linkName string, out *raw.EntryOut) (code Status)
	Readlink(header *raw.InHeader) (out []byte, code Status)
	Access(input *raw.AccessIn) (code Status)

	// Extended attributes.
	GetXAttrSize(header *raw.InHeader, attr string) (sz int, code Status)
	GetXAttrData(header *raw.InHeader, attr string) (data []byte, code Status)
	ListXAttr(header *raw.InHeader) (attributes []byte, code Status)
	SetXAttr(input *raw.SetXAttrIn, attr string, data []byte) Status
	RemoveXAttr(header *raw.InHeader, attr string) (code Status)

	// File handling.
	Create(input *raw.CreateIn, name string, out *raw.CreateOut) (code Status)
	Open(input *raw.OpenIn, out *raw.OpenOut) (status Status)
	Read(input *raw.ReadIn, buf []byte) (ReadResult, Status)

	Release(input *raw.ReleaseIn)
	Write(input *raw.WriteIn, data []byte) (written uint32, code Status)
	Flush(input *raw.FlushIn) Status
	Fsync(input *raw.FsyncIn) (code Status)
	Fallocate(input *raw.FallocateIn) (code Status)

	// Directory handling
	OpenDir(input *raw.OpenIn, out *raw.OpenOut) (status Status)
	ReadDir(input *raw.ReadIn, out *DirEntryList) Status
	ReadDirPlus(input *raw.ReadIn, out *DirEntryList) Status
	ReleaseDir(input *raw.ReleaseIn)
	FsyncDir(input *raw.FsyncIn) (code Status)

	//
	StatFs(input *raw.InHeader, out *raw.StatfsOut) (code Status)

	// This is called on processing the first request. The
	// filesystem implementation can use the server argument to
	// talk back to the kernel (through notify methods).
	Init(*Server)
}
